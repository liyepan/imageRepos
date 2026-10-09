package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type uploadError struct {
	code int
	msg  string
}

func (e *uploadError) Error() string { return e.msg }

func uploadStatus(err error) int {
	var ue *uploadError
	if errors.As(err, &ue) {
		return ue.code
	}
	return http.StatusInternalServerError
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"success": false, "message": msg})
}

func readBody(r *http.Request, limit int64) []byte {
	if r.Body == nil {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(r.Body, limit))
	return b
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, &uploadError{
			code: http.StatusRequestEntityTooLarge,
			msg:  "单个文件超过 " + strconv.FormatInt(max>>20, 10) + " MB 限制",
		}
	}
	return data, nil
}

// formValue 一个参数可能来自 ?query=、JSON body、普通表单或 multipart 表单。
// 前端删除用的是 FormData（multipart），脚本多半用 JSON，所以三种都得认。
func formValue(r *http.Request, key string) string {
	if v := r.URL.Query().Get(key); v != "" {
		return v
	}
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		var m map[string]any
		if json.Unmarshal(readBody(r, 16384), &m) == nil {
			if s, ok := m[key].(string); ok {
				return s
			}
		}
		return ""
	}
	// 注意：不能先调 ParseForm()。它对 multipart 是空操作，
	// 却会把 r.PostForm 置为非 nil，导致 PostFormValue 再也不去解析 multipart。
	_ = r.ParseMultipartForm(1 << 20)
	return r.PostFormValue(key)
}

func serveHTML(w http.ResponseWriter, page []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(page)
}

func (s *Server) serveLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth.loggedIn(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	serveHTML(w, s.pageLogin)
}

func (s *Server) serveHome(w http.ResponseWriter, r *http.Request) {
	if !s.auth.loggedIn(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	serveHTML(w, s.pageHome)
}

// publicURL 生成外链。优先用 PUBLIC_BASE，否则按请求头猜。
func (s *Server) publicURL(r *http.Request, name string) string {
	base := s.cfg.PublicBase
	if base == "" {
		scheme := "http"
		if isHTTPS(r) {
			scheme = "https"
		}
		host := r.Host
		if h := r.Header.Get("X-Forwarded-Host"); h != "" {
			host = strings.TrimSpace(strings.Split(h, ",")[0])
		}
		base = scheme + "://" + host
	}
	return base + "/i/" + url.PathEscape(name)
}

func (s *Server) imageJSON(r *http.Request, img *Image) map[string]any {
	u := s.publicURL(r, img.Name)
	alt := html.EscapeString(img.Original)
	return map[string]any{
		"name":     img.Name,
		"original": img.Original,
		"url":      u,
		"markdown": "![" + img.Original + "](" + u + ")",
		"html":     "<img src=\"" + u + "\" alt=\"" + alt + "\">",
		"size":     img.Size,
		"width":    img.Width,
		"height":   img.Height,
		"mime":     img.MIME,
		"time":     img.At.Format(time.RFC3339),
	}
}

func (s *Server) apiLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	ip := clientIP(r)
	if d := s.auth.lockedFor(ip); d > 0 {
		writeErr(w, http.StatusTooManyRequests,
			"密码错误次数过多，请 "+strconv.Itoa(int(d.Seconds())+1)+" 秒后再试")
		return
	}

	password := formValue(r, "password")
	if password == "" || !constEq(password, s.cfg.Password) {
		s.auth.recordFail(ip)
		time.Sleep(300 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, "密码错误")
		return
	}
	s.auth.clearFails(ip)
	s.auth.setCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) apiLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.clearCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// apiToken 只在网页登录态下返回 Token，避免 Token 自己泄露自己
func (s *Server) apiToken(w http.ResponseWriter, r *http.Request) {
	if !s.auth.loggedIn(r) {
		writeErr(w, http.StatusUnauthorized, "未登录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "token": s.cfg.APIToken})
}

func (s *Server) apiList(w http.ResponseWriter, r *http.Request) {
	if !s.auth.authorized(r) {
		writeErr(w, http.StatusUnauthorized, "未授权")
		return
	}
	q := r.URL.Query().Get("q")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 || size > 200 {
		size = 60
	}

	items, total := s.store.List(q, (page-1)*size, size)
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, s.imageJSON(r, it))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"total":   total,
		"page":    page,
		"size":    size,
		"images":  out,
	})
}

func (s *Server) apiDelete(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if !s.auth.authorized(r) {
		writeErr(w, http.StatusUnauthorized, "未授权")
		return
	}
	name := formValue(r, "name")
	if !safeName(name) {
		writeErr(w, http.StatusBadRequest, "文件名不合法")
		return
	}
	ok, err := s.store.Delete(name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "删除失败: "+err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "图片不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "name": name})
}

func (s *Server) apiUpload(w http.ResponseWriter, r *http.Request) {
	if !s.auth.authorized(r) {
		writeErr(w, http.StatusUnauthorized,
			"未授权：请先登录，或带上 Authorization: Bearer <API Token>")
		return
	}

	// 一次可能传多张，总上限给宽一点；单文件限制在 saveImage 里卡
	total := s.cfg.MaxBytes * 50
	if total < 512<<20 {
		total = 512 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, total)

	ct := strings.ToLower(r.Header.Get("Content-Type"))
	var (
		results []map[string]any
		err     error
	)
	if strings.HasPrefix(ct, "multipart/form-data") {
		results, err = s.handleMultipart(r)
	} else {
		results, err = s.handleRaw(r)
	}

	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeErr(w, http.StatusRequestEntityTooLarge, "请求体过大")
			return
		}
		log.Printf("上传失败: %v", err)
		writeErr(w, uploadStatus(err), err.Error())
		return
	}
	if len(results) == 0 {
		writeErr(w, http.StatusBadRequest, "没有收到任何图片文件")
		return
	}

	// ?format=text 直接返回纯文本 URL，方便脚本 / 自定义上传器
	if f := r.URL.Query().Get("format"); f == "text" || f == "plain" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		for _, res := range results {
			if u, ok := res["url"].(string); ok {
				_, _ = w.Write([]byte(u + "\n"))
			}
		}
		return
	}

	resp := map[string]any{"success": true, "files": results, "count": len(results)}
	// 把第一张图提到顶层，PicGo / ShareX 配 json path 时直接写 url 就行
	for k, v := range results[0] {
		if k != "files" {
			resp[k] = v
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMultipart(r *http.Request) ([]map[string]any, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, &uploadError{http.StatusBadRequest, "解析上传数据失败: " + err.Error()}
	}
	var out []map[string]any
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, &uploadError{http.StatusBadRequest, "读取上传数据失败: " + err.Error()}
		}

		filename := part.FileName()
		field := part.FormName()
		if filename == "" && field != "file" && field != "files" && field != "image" {
			_ = part.Close()
			continue
		}

		data, err := readLimited(part, s.cfg.MaxBytes)
		_ = part.Close()
		if err != nil {
			return out, err
		}
		if len(data) == 0 {
			continue
		}
		res, err := s.saveImage(r, data, filename)
		if err != nil {
			return out, err
		}
		out = append(out, res)
	}
	return out, nil
}

// handleRaw 支持直接把图片字节当请求体发上来（ShareX 之类经常这么干）
func (s *Server) handleRaw(r *http.Request) ([]map[string]any, error) {
	data, err := readLimited(r.Body, s.cfg.MaxBytes)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		if _, params, perr := mime.ParseMediaType(r.Header.Get("Content-Disposition")); perr == nil {
			name = params["filename"]
		}
	}
	if name == "" {
		if ct := r.Header.Get("Content-Type"); strings.HasPrefix(ct, "image/") {
			name = "image." + strings.TrimPrefix(strings.Split(ct, ";")[0], "image/")
		}
	}

	res, err := s.saveImage(r, data, name)
	if err != nil {
		return nil, err
	}
	return []map[string]any{res}, nil
}

func (s *Server) saveImage(r *http.Request, data []byte, original string) (map[string]any, error) {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	mimeType, ext, ok := sniffMIME(head)
	if !ok {
		return nil, &uploadError{
			code: http.StatusUnsupportedMediaType,
			msg:  "不是支持的图片格式（支持 jpg / png / gif / webp / bmp / avif）",
		}
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	// 同一张图重复上传就直接复用，不占额外空间
	if exist := s.store.ByHash(hash); exist != nil {
		if _, err := os.Stat(filepath.Join(s.cfg.FilesDir, exist.Name)); err == nil {
			return s.imageJSON(r, exist), nil
		}
	}

	if original == "" {
		original = "image" + ext
	}
	name := slug(original) + "-" + hash[:8] + ext
	if !safeName(name) {
		name = "image-" + hash[:8] + ext
	}

	width, height := dimensions(mimeType, data)

	dst := filepath.Join(s.cfg.FilesDir, name)
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return nil, &uploadError{http.StatusInternalServerError, "写入文件失败: " + err.Error()}
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return nil, &uploadError{http.StatusInternalServerError, "保存文件失败: " + err.Error()}
	}

	img := &Image{
		Name:     name,
		Original: original,
		MIME:     mimeType,
		Size:     int64(len(data)),
		Width:    width,
		Height:   height,
		Hash:     hash,
		At:       time.Now(),
	}
	if err := s.store.Add(img); err != nil {
		log.Printf("写入索引失败: %v", err)
	}
	return s.imageJSON(r, img), nil
}

func (s *Server) serveImage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !safeName(name) {
		http.NotFound(w, r)
		return
	}
	img := s.store.ByName(name)
	if img == nil {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(filepath.Join(s.cfg.FilesDir, name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", img.MIME)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// 文件名里带内容哈希，内容永不改变，可以放心长缓存
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, name, st.ModTime(), f)
}

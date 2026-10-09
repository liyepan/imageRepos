package main

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
)

// 白名单：只有这里面的类型会被接受
var mimeToExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"image/bmp":  ".bmp",
	"image/avif": ".avif",
}

// sniffMIME 只看文件头判断真实类型。
// 认不出来就拒绝 —— 绝不回退到看扩展名，否则把木马改成 .png 就能传上来。
// 返回的 MIME 一定来自白名单。
func sniffMIME(head []byte) (string, string, bool) {
	ct := http.DetectContentType(head)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ext, ok := mimeToExt[ct]; ok {
		return ct, ext, true
	}
	// AVIF 不在标准库的嗅探表里，手动认一下 ISO-BMFF 的 brand
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		switch string(head[8:12]) {
		case "avif", "avis":
			return "image/avif", ".avif", true
		}
	}
	return "", "", false
}

// safeName 存储文件名只允许 ASCII 字母数字和 . - _，
// 从根上堵死路径穿越
func safeName(n string) bool {
	if n == "" || len(n) > 128 || strings.HasPrefix(n, ".") {
		return false
	}
	if strings.Contains(n, "..") {
		return false
	}
	for _, c := range n {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// slug 把原始文件名变成 URL 友好的前缀，非 ASCII 一律丢掉
func slug(s string) string {
	s = strings.ToLower(s)
	if i := strings.LastIndexByte(s, '.'); i > 0 {
		s = s[:i]
	}
	var b strings.Builder
	lastDash := false
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
			lastDash = false
		case c == '-' || c == '_' || c == ' ' || c == '.' || c == '(' || c == ')':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	if out == "" {
		out = "image"
	}
	return out
}

func dimensions(mimeType string, data []byte) (int, int) {
	switch mimeType {
	case "image/webp":
		return webpSize(data)
	case "image/jpeg", "image/png", "image/gif":
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			return cfg.Width, cfg.Height
		}
	}
	return 0, 0
}

// webpSize 标准库不解码 webp，这里直接读文件头
func webpSize(b []byte) (int, int) {
	if len(b) < 30 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0, 0
	}
	switch string(b[12:16]) {
	case "VP8X":
		w := int(uint32(b[24])|uint32(b[25])<<8|uint32(b[26])<<16) + 1
		h := int(uint32(b[27])|uint32(b[28])<<8|uint32(b[29])<<16) + 1
		return w, h
	case "VP8 ":
		w := int(binary.LittleEndian.Uint16(b[26:28]) & 0x3fff)
		h := int(binary.LittleEndian.Uint16(b[28:30]) & 0x3fff)
		return w, h
	case "VP8L":
		bits := binary.LittleEndian.Uint32(b[21:25])
		w := int(bits&0x3fff) + 1
		h := int((bits>>14)&0x3fff) + 1
		return w, h
	}
	return 0, 0
}

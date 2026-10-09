package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

const version = "1.0.0"

type Config struct {
	Addr       string
	DataDir    string
	FilesDir   string
	Password   string
	APIToken   string
	PublicBase string
	MaxBytes   int64
}

type Server struct {
	cfg       *Config
	store     *Store
	auth      *Auth
	pageHome  []byte
	pageLogin []byte
}

func envStr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func loadOrCreateSecret(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		return b[:32], nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return b, nil
}

func loadOrCreateToken(path string) (string, bool, error) {
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, false, nil
		}
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", false, err
	}
	s := hex.EncodeToString(b)
	if err := os.WriteFile(path, []byte(s+"\n"), 0o600); err != nil {
		return "", false, err
	}
	return s, true, nil
}

func main() {
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[imghost] ")

	dataDir := envStr("DATA_DIR", "./data")
	filesDir := filepath.Join(dataDir, "files")

	cfg := &Config{
		Addr:       envStr("ADDR", ":8080"),
		DataDir:    dataDir,
		FilesDir:   filesDir,
		Password:   os.Getenv("PASSWORD"),
		PublicBase: strings.TrimRight(envStr("PUBLIC_BASE", ""), "/"),
		MaxBytes:   int64(envInt("MAX_MB", 20)) << 20,
	}

	if cfg.Password == "" {
		log.Fatal("缺少环境变量 PASSWORD —— 请设置登录密码后重试")
	}
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		log.Fatalf("无法创建数据目录 %s: %v", filesDir, err)
	}

	secret, err := loadOrCreateSecret(filepath.Join(dataDir, ".secret"))
	if err != nil {
		log.Fatalf("初始化会话密钥失败: %v", err)
	}

	token := strings.TrimSpace(os.Getenv("API_TOKEN"))
	tokenGenerated := false
	if token == "" {
		token, tokenGenerated, err = loadOrCreateToken(filepath.Join(dataDir, ".apitoken"))
		if err != nil {
			log.Fatalf("初始化 API Token 失败: %v", err)
		}
	}
	cfg.APIToken = token

	store, err := NewStore(filesDir, filepath.Join(dataDir, "index.json"))
	if err != nil {
		log.Fatalf("读取索引失败: %v", err)
	}

	home, err := webFS.ReadFile("web/index.html")
	if err != nil {
		log.Fatalf("内置页面缺失: %v", err)
	}
	login, err := webFS.ReadFile("web/login.html")
	if err != nil {
		log.Fatalf("内置页面缺失: %v", err)
	}

	s := &Server{
		cfg:       cfg,
		store:     store,
		auth:      NewAuth(secret, cfg.Password, token),
		pageHome:  home,
		pageLogin: login,
	}

	addr := cfg.Addr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	log.Printf("imghost %s", version)
	log.Printf("  数据目录  : %s", cfg.DataDir)
	log.Printf("  图片目录  : %s", cfg.FilesDir)
	log.Printf("  单文件上限: %d MB", cfg.MaxBytes>>20)
	log.Printf("  已有图片  : %d 张", store.Count())
	if n := store.Missing(); n > 0 {
		log.Printf("  ⚠️  有 %d 条索引对应的文件不存在（数据卷没挂上？）", n)
	}
	if cfg.PublicBase != "" {
		log.Printf("  图片域名  : %s", cfg.PublicBase)
	}
	if tokenGenerated {
		log.Printf("  API Token : %s", token)
		log.Printf("              （已写入 %s，也可以在网页右上角查看）", filepath.Join(dataDir, ".apitoken"))
	}
	log.Printf("  启动完成  → http://%s", addr)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /login", s.serveLogin)
	mux.HandleFunc("GET /{$}", s.serveHome)
	mux.HandleFunc("POST /api/login", s.apiLogin)
	mux.HandleFunc("POST /api/logout", s.apiLogout)
	mux.HandleFunc("GET /api/list", s.apiList)
	mux.HandleFunc("GET /api/token", s.apiToken)
	mux.HandleFunc("POST /api/upload", s.apiUpload)
	mux.HandleFunc("POST /api/delete", s.apiDelete)
	mux.HandleFunc("GET /i/{name}", s.serveImage)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.logRequests(mux),
		ReadHeaderTimeout: 20 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Print("正在关闭…")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		// 图片请求太多，不记日志
		if !strings.HasPrefix(r.URL.Path, "/i/") && r.URL.Path != "/healthz" {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

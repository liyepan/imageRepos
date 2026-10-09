package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cookieName = "imageRepos_session"
	sessionTTL = 30 * 24 * time.Hour
	maxFails   = 8
	lockout    = 5 * time.Minute
)

type Auth struct {
	secret   []byte
	password string
	token    string

	mu    sync.Mutex
	fails map[string]*failRec
}

type failRec struct {
	count int
	until time.Time
}

func NewAuth(secret []byte, password, token string) *Auth {
	return &Auth{
		secret:   secret,
		password: password,
		token:    token,
		fails:    map[string]*failRec{},
	}
}

// constEq 定长比较，避免时序侧信道
func constEq(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

func (a *Auth) sign(payload string) string {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte(payload))
	return hex.EncodeToString(m.Sum(nil))
}

func (a *Auth) newSession() string {
	exp := strconv.FormatInt(time.Now().Add(sessionTTL).Unix(), 10)
	return exp + "." + a.sign(exp)
}

func (a *Auth) validSession(tok string) bool {
	i := strings.IndexByte(tok, '.')
	if i <= 0 || i >= len(tok)-1 {
		return false
	}
	payload, sig := tok[:i], tok[i+1:]
	if !hmac.Equal([]byte(sig), []byte(a.sign(payload))) {
		return false
	}
	exp, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}

func (a *Auth) loggedIn(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return false
	}
	return a.validSession(c.Value)
}

func (a *Auth) tokenFrom(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
			return strings.TrimSpace(h[7:])
		}
	}
	if t := r.Header.Get("X-Api-Token"); t != "" {
		return strings.TrimSpace(t)
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func (a *Auth) tokenOK(r *http.Request) bool {
	t := a.tokenFrom(r)
	return t != "" && a.token != "" && constEq(t, a.token)
}

// authorized：网页会话或 API Token，任意一个通过即可
func (a *Auth) authorized(r *http.Request) bool {
	return a.loggedIn(r) || a.tokenOK(r)
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	p := r.Header.Get("X-Forwarded-Proto")
	if i := strings.IndexByte(p, ','); i >= 0 {
		p = p[:i]
	}
	return strings.EqualFold(strings.TrimSpace(p), "https")
}

func (a *Auth) setCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    a.newSession(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
		MaxAge:   int(sessionTTL / time.Second),
	})
}

func (a *Auth) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *Auth) lockedFor(ip string) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	rec := a.fails[ip]
	if rec == nil || !time.Now().Before(rec.until) {
		return 0
	}
	return time.Until(rec.until)
}

func (a *Auth) recordFail(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rec := a.fails[ip]
	if rec == nil {
		rec = &failRec{}
		a.fails[ip] = rec
	}
	rec.count++
	if rec.count >= maxFails {
		rec.until = time.Now().Add(lockout)
		rec.count = 0
	}
}

func (a *Auth) clearFails(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.fails, ip)
}

package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const contentSecurityPolicy = "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

func (a *App) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		if a.cfg.WriteToken != "" && writeProtected(r) && !a.authorized(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func setSecurityHeaders(w http.ResponseWriter) {
	header := w.Header()
	header.Set("Content-Security-Policy", contentSecurityPolicy)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Cache-Control", "no-store")
}

func writeProtected(r *http.Request) bool {
	path := r.URL.Path
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	case http.MethodGet, http.MethodHead:
		if path == "/ca.crt" || path == "/api/bench/probe" {
			return true
		}
		if strings.HasPrefix(path, "/api/blobs/") {
			return true
		}
	}
	return false
}

func (a *App) authorized(r *http.Request) bool {
	token := a.cfg.WriteToken
	if token == "" {
		return true
	}
	got := bearerToken(r.Header.Get("Authorization"))
	if len(got) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteProtected(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{method: http.MethodGet, path: "/healthz", want: false},
		{method: http.MethodGet, path: "/api/info", want: false},
		{method: http.MethodGet, path: "/api/results", want: false},
		{method: http.MethodGet, path: "/api/blobs", want: false},
		{method: http.MethodGet, path: "/", want: false},
		{method: http.MethodGet, path: "/ca.crt", want: true},
		{method: http.MethodGet, path: "/api/bench/probe", want: true},
		{method: http.MethodGet, path: "/api/blobs/cap.bin", want: true},
		{method: http.MethodHead, path: "/api/blobs/cap.bin", want: true},
		{method: http.MethodPost, path: "/api/blobs", want: true},
		{method: http.MethodPut, path: "/api/blobs/cap.bin", want: true},
		{method: http.MethodDelete, path: "/api/blobs/cap.bin", want: true},
		{method: http.MethodPost, path: "/api/results/timings", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if got := writeProtected(req); got != tt.want {
				t.Fatalf("writeProtected(%s %s)=%v want %v", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "empty", header: "", want: ""},
		{name: "basic", header: "Basic abc", want: ""},
		{name: "bearer", header: "Bearer secret-token", want: "secret-token"},
		{name: "bearer spaces", header: "Bearer  secret-token  ", want: "secret-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bearerToken(tt.header); got != tt.want {
				t.Fatalf("bearerToken(%q)=%q want %q", tt.header, got, tt.want)
			}
		})
	}
}

package tlslisten

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/certs"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
)

func TestHandshakeDurationOnFirstRequest(t *testing.T) {
	dir := t.TempDir()
	material, err := certs.LoadOrGenerate(config.Config{
		TLSCertFile: filepath.Join(dir, "tls.crt"),
		TLSKeyFile:  filepath.Join(dir, "tls.key"),
		TLSCAFile:   filepath.Join(dir, "ca.crt"),
		TLSDNSNames: []string{"localhost"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tlsCfg := certs.ServerTLSConfig(material, nil)
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tracker := NewTracker()
	ln := New(inner, tlsCfg, tracker)
	var gotMs float64
	var gotReused bool
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMs, gotReused = FromContext(r.Context()).ConsumeHandshake()
			_, _ = w.Write([]byte("ok"))
		}),
		ReadHeaderTimeout: time.Second,
		ConnContext:       tracker.ConnContext,
		ConnState:         tracker.ConnState,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}
	url := "https://" + inner.Addr().String() + "/"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if gotMs <= 0 {
		t.Fatalf("expected handshake ms, got %v reused=%v", gotMs, gotReused)
	}
	if gotReused {
		t.Fatal("first request must not be reused")
	}

	bad := &tls.Config{InsecureSkipVerify: false, ServerName: "wrong.example"}
	conn, err := tls.Dial("tcp", inner.Addr().String(), bad)
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected handshake failure")
	}

	resp, err = client.Get(url)
	if err != nil {
		t.Fatalf("server must keep serving after a failed handshake: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

package tlslisten

import (
	"crypto/tls"
	"errors"
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
	tlsCfg := certs.ServerTLSConfig(material, nil, config.Config{DisableSessionTickets: true})
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
			if _, err := w.Write([]byte("ok")); err != nil {
				t.Errorf("write: %v", err)
			}
		}),
		ReadHeaderTimeout: time.Second,
		ConnContext:       tracker.ConnContext,
		ConnState:         tracker.ConnState,
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- srv.Serve(ln)
	}()
	t.Cleanup(func() {
		closeErr := srv.Close()
		serveErr := <-serveDone
		if closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			t.Errorf("close: %v", closeErr)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			t.Errorf("serve: %v", serveErr)
		}
	})

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
	if err := drain(resp); err != nil {
		t.Fatal(err)
	}
	if gotMs <= 0 {
		t.Fatalf("expected handshake ms, got %v reused=%v", gotMs, gotReused)
	}
	if gotReused {
		t.Fatal("first request must not be reused")
	}

	bad := &tls.Config{InsecureSkipVerify: false, ServerName: "wrong.example"}
	conn, err := tls.Dial("tcp", inner.Addr().String(), bad)
	if err == nil {
		if closeErr := conn.Close(); closeErr != nil {
			t.Errorf("close unexpected conn: %v", closeErr)
		}
		t.Fatal("expected handshake failure")
	}

	resp, err = client.Get(url)
	if err != nil {
		t.Fatalf("server must keep serving after a failed handshake: %v", err)
	}
	if err := drain(resp); err != nil {
		t.Fatal(err)
	}
}

func drain(resp *http.Response) error {
	_, copyErr := io.Copy(io.Discard, resp.Body)
	closeErr := resp.Body.Close()
	return errors.Join(copyErr, closeErr)
}

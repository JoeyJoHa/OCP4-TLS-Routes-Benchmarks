package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/blobs"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/certs"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/results"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		TLSCertFile:  filepath.Join(dir, "tls.crt"),
		TLSKeyFile:   filepath.Join(dir, "tls.key"),
		TLSCAFile:    filepath.Join(dir, "ca.crt"),
		TLSDNSNames:  []string{"localhost"},
		DataDir:      dir,
		MaxBlobBytes: 1024 * 1024,
		ResultsLog:   filepath.Join(dir, "results", "runs.jsonl"),
	}
	material, err := certs.LoadOrGenerate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := blobs.NewStore(dir, cfg.MaxBlobBytes, bytes.NewReader(bytes.Repeat([]byte("r"), 8192)))
	if err != nil {
		t.Fatal(err)
	}
	logger, err := results.NewLogger(cfg.ResultsLog)
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, store, logger, material)
}

func TestHealthAndInfoHTTPVsTLS(t *testing.T) {
	app := newTestApp(t)
	plain := httptest.NewServer(app.Handler())
	t.Cleanup(plain.Close)

	resp, err := http.Get(plain.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz=%d", resp.StatusCode)
	}

	info := getInfo(t, plain.Client(), plain.URL+"/api/info")
	if info["tls"] == true {
		t.Fatalf("plain HTTP should not report pod TLS: %v", info)
	}

	tlsServer := httptest.NewTLSServer(app.Handler())
	t.Cleanup(tlsServer.Close)

	tlsInfo := getInfo(t, tlsServer.Client(), tlsServer.URL+"/api/info")
	if tlsInfo["tls"] != true {
		t.Fatalf("HTTPS should report pod TLS: %v", tlsInfo)
	}
}

func TestBlobGenerateUploadDownloadAndResults(t *testing.T) {
	app := newTestApp(t)
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	client := srv.Client()

	resp, err := client.Post(srv.URL+"/api/blobs?name=bench.bin&size=4096", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("generate=%d %s", resp.StatusCode, body)
	}

	payload := bytes.Repeat([]byte("u"), 2048)
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/blobs/from-client.bin", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = int64(len(payload))
	putResp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(putResp.Body)
		t.Fatalf("upload=%d %s", putResp.StatusCode, body)
	}

	getResp, err := client.Get(srv.URL + "/api/blobs/from-client.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	got, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("downloaded bytes do not match upload")
	}

	resultsResp, err := client.Get(srv.URL + "/api/results")
	if err != nil {
		t.Fatal(err)
	}
	defer resultsResp.Body.Close()
	var payloadJSON struct {
		Runs []results.Run `json:"runs"`
	}
	if err := json.NewDecoder(resultsResp.Body).Decode(&payloadJSON); err != nil {
		t.Fatal(err)
	}
	if len(payloadJSON.Runs) < 3 {
		t.Fatalf("expected generate+upload+download rows, got %d", len(payloadJSON.Runs))
	}

	ca, err := client.Get(srv.URL + "/ca.crt")
	if err != nil {
		t.Fatal(err)
	}
	defer ca.Body.Close()
	pem, _ := io.ReadAll(ca.Body)
	if !bytes.Contains(pem, []byte("BEGIN CERTIFICATE")) {
		t.Fatal("ca.crt missing PEM")
	}
}

func TestRejectsInvalidBlobName(t *testing.T) {
	app := newTestApp(t)
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	resp, err := srv.Client().Post(srv.URL+"/api/blobs?name=../secret&size=8", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func getInfo(t *testing.T, client *http.Client, url string) map[string]any {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	return info
}

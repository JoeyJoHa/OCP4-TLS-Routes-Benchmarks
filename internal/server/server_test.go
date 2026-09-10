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

func mustHandler(t *testing.T, app *App) http.Handler {
	t.Helper()
	handler, err := app.Handler()
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func TestHealthAndInfoHTTPVsTLS(t *testing.T) {
	app := newTestApp(t)
	plain := httptest.NewServer(mustHandler(t, app))
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

	tlsServer := httptest.NewTLSServer(mustHandler(t, app))
	t.Cleanup(tlsServer.Close)

	tlsInfo := getInfo(t, tlsServer.Client(), tlsServer.URL+"/api/info")
	if tlsInfo["tls"] != true {
		t.Fatalf("HTTPS should report pod TLS: %v", tlsInfo)
	}
	if tlsInfo["server_cert_key_algorithm"] != "ECDSA" {
		t.Fatalf("expected ECDSA server key, got %v", tlsInfo["server_cert_key_algorithm"])
	}
	if tlsInfo["server_cert_key_size"] != float64(256) {
		t.Fatalf("expected 256-bit key, got %v", tlsInfo["server_cert_key_size"])
	}
	if tlsInfo["cipher"] == nil || tlsInfo["cipher"] == "" {
		t.Fatalf("HTTPS should report cipher: %v", tlsInfo)
	}
}

func TestBlobGenerateUploadDownloadAndResults(t *testing.T) {
	app := newTestApp(t)
	srv := httptest.NewServer(mustHandler(t, app))
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
	for _, run := range payloadJSON.Runs {
		if run.TLS {
			t.Fatalf("HTTP server run should not set TLS: %+v", run)
		}
		if run.TLSKeyAlgorithm != "" || run.Cipher != "" {
			t.Fatalf("HTTP run should omit TLS key and cipher: %+v", run)
		}
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

func TestTLSUploadRecordsCipherAndKey(t *testing.T) {
	app := newTestApp(t)
	srv := httptest.NewTLSServer(mustHandler(t, app))
	t.Cleanup(srv.Close)

	payload := bytes.Repeat([]byte("t"), 1024)
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/blobs/tls-upload.bin", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = int64(len(payload))
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload=%d %s", resp.StatusCode, body)
	}

	resultsResp, err := srv.Client().Get(srv.URL + "/api/results")
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
	if len(payloadJSON.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(payloadJSON.Runs))
	}
	run := payloadJSON.Runs[0]
	if !run.TLS {
		t.Fatalf("expected TLS run: %+v", run)
	}
	if run.Cipher == "" {
		t.Fatalf("expected cipher: %+v", run)
	}
	if run.TLSKeyAlgorithm != "ECDSA" || run.TLSKeySize != 256 || run.TLSKeyCurve != "P-256" {
		t.Fatalf("expected ECDSA P-256, got %+v", run)
	}
}

func TestAttachClientTimingsMergesCurlPhases(t *testing.T) {
	app := newTestApp(t)
	srv := httptest.NewServer(mustHandler(t, app))
	t.Cleanup(srv.Close)

	payload := bytes.Repeat([]byte("t"), 512)
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/blobs/timed.bin", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = int64(len(payload))
	putResp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("upload=%d", putResp.StatusCode)
	}

	body := bytes.NewBufferString(`{
		"operation":"upload",
		"name":"timed.bin",
		"time_namelookup":0.001,
		"time_connect":0.004,
		"time_appconnect":0.054,
		"time_pretransfer":0.055,
		"time_starttransfer":0.060,
		"time_total":0.160
	}`)
	timingResp, err := srv.Client().Post(srv.URL+"/api/results/timings", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer timingResp.Body.Close()
	if timingResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(timingResp.Body)
		t.Fatalf("timings=%d %s", timingResp.StatusCode, raw)
	}

	resultsResp, err := srv.Client().Get(srv.URL + "/api/results")
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
	if len(payloadJSON.Runs) != 1 {
		t.Fatalf("len=%d", len(payloadJSON.Runs))
	}
	run := payloadJSON.Runs[0]
	if run.DNSMs != 1 || run.TCPConnectMs != 3 || run.TLSHandshakeMs != 50 || run.TTFBMs != 5 || run.TransferMs != 100 {
		t.Fatalf("phases=%+v", run)
	}
}

func TestAttachClientTimingsRequiresExistingRun(t *testing.T) {
	app := newTestApp(t)
	srv := httptest.NewServer(mustHandler(t, app))
	t.Cleanup(srv.Close)
	resp, err := srv.Client().Post(srv.URL+"/api/results/timings", "application/json", bytes.NewBufferString(`{"operation":"upload","name":"missing.bin","time_total":0.1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestBenchProbeAndExperimentMerge(t *testing.T) {
	app := newTestApp(t)
	srv := httptest.NewTLSServer(mustHandler(t, app))
	t.Cleanup(srv.Close)

	probeURL := srv.URL + "/api/bench/probe?experiment_id=exp1&sample_index=1&route_mode=passthrough"
	resp, err := srv.Client().Get(probeURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("probe=%d %s", resp.StatusCode, body)
	}

	body := bytes.NewBufferString(`{
		"operation":"handshake",
		"experiment_id":"exp1",
		"sample_index":1,
		"route_mode":"passthrough",
		"time_namelookup":0.001,
		"time_connect":0.004,
		"time_appconnect":0.054,
		"time_pretransfer":0.055,
		"time_starttransfer":0.060,
		"time_total":0.060
	}`)
	timingResp, err := srv.Client().Post(srv.URL+"/api/results/timings", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer timingResp.Body.Close()
	if timingResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(timingResp.Body)
		t.Fatalf("timings=%d %s", timingResp.StatusCode, raw)
	}

	resultsResp, err := srv.Client().Get(srv.URL + "/api/results")
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
	if len(payloadJSON.Runs) != 1 {
		t.Fatalf("len=%d", len(payloadJSON.Runs))
	}
	run := payloadJSON.Runs[0]
	if run.Operation != "handshake" || run.ExperimentID != "exp1" || run.SampleIndex != 1 {
		t.Fatalf("run=%+v", run)
	}
	if run.TLSHandshakeMs != 50 {
		t.Fatalf("client handshake=%+v", run)
	}
	if run.RouteMode != "passthrough" {
		t.Fatalf("route_mode=%q", run.RouteMode)
	}
}

func TestRejectsInvalidBlobName(t *testing.T) {
	app := newTestApp(t)
	srv := httptest.NewServer(mustHandler(t, app))
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

func TestBenchProbeRejectsInvalidMetadata(t *testing.T) {
	app := newTestApp(t)
	srv := httptest.NewServer(mustHandler(t, app))
	t.Cleanup(srv.Close)
	tests := []struct {
		name string
		url  string
	}{
		{name: "missing id", url: "/api/bench/probe?sample_index=1"},
		{name: "html id", url: "/api/bench/probe?experiment_id=%3Cscript%3E&sample_index=1"},
		{name: "bad route", url: "/api/bench/probe?experiment_id=exp1&sample_index=1&route_mode=ftp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := srv.Client().Get(srv.URL + tt.url)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status=%d", resp.StatusCode)
			}
		})
	}
}

func TestAttachClientTimingsRejectsOversizedJSON(t *testing.T) {
	app := newTestApp(t)
	srv := httptest.NewServer(mustHandler(t, app))
	t.Cleanup(srv.Close)
	body := bytes.Repeat([]byte("n"), config.MaxJSONBodyBytes+1)
	resp, err := srv.Client().Post(srv.URL+"/api/results/timings", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge && resp.StatusCode != http.StatusBadRequest {
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

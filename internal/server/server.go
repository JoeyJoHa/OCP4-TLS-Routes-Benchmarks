package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/blobs"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/certs"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/requestinfo"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/results"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/web"
)

// App is the HTTP/HTTPS TLS target.
type App struct {
	cfg      config.Config
	store    *blobs.Store
	log      *results.Logger
	material certs.Material
	hostname string
	ready    atomic.Bool
}

func New(cfg config.Config, store *blobs.Store, logger *results.Logger, material certs.Material) *App {
	host, err := os.Hostname()
	if err != nil {
		host = "tlsbench"
	}
	app := &App{
		cfg:      cfg,
		store:    store,
		log:      logger,
		material: material,
		hostname: host,
	}
	app.ready.Store(true)
	return app
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.healthz)
	mux.HandleFunc("GET /readyz", a.readyz)
	mux.HandleFunc("GET /ca.crt", a.serveCA)
	mux.HandleFunc("GET /api/info", a.apiInfo)
	mux.HandleFunc("GET /api/results", a.apiResults)
	mux.HandleFunc("GET /api/blobs", a.listBlobs)
	mux.HandleFunc("POST /api/blobs", a.generateBlob)
	mux.HandleFunc("GET /api/blobs/{name}", a.downloadBlob)
	mux.HandleFunc("PUT /api/blobs/{name}", a.uploadBlob)
	mux.HandleFunc("DELETE /api/blobs/{name}", a.deleteBlob)
	mux.HandleFunc("GET /{$}", a.dashboard)

	staticFS, err := fs.Sub(web.Files, "static")
	if err != nil {
		log.Fatalf("embed static files: %v", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return mux
}

func (a *App) ListenAndServe(ctx context.Context) error {
	handler := a.Handler()
	httpSrv := newHTTPServer(a.cfg.HTTPAddr, handler)
	httpsSrv := newHTTPServer(a.cfg.HTTPSAddr, handler)

	clientCAs, err := certs.LoadClientCAs(a.cfg.TLSClientCAFile)
	if err != nil {
		return err
	}
	httpsSrv.TLSConfig = certs.ServerTLSConfig(a.material, clientCAs)

	errCh := make(chan error, 2)
	go func() {
		log.Printf("HTTP listening on %s", a.cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http: %w", err)
		}
	}()
	go func() {
		log.Printf("HTTPS listening on %s", a.cfg.HTTPSAddr)
		if err := httpsSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("https: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
		_ = httpsSrv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: time.Duration(config.ReadHeaderTimeoutSecs) * time.Second,
	}
}

func (a *App) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (a *App) readyz(w http.ResponseWriter, _ *http.Request) {
	if !a.ready.Load() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (a *App) serveCA(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", "attachment; filename=ca.crt")
	_, _ = w.Write(a.material.CAPEM)
}

func (a *App) dashboard(w http.ResponseWriter, _ *http.Request) {
	data, err := web.Files.ReadFile("templates/index.html")
	if err != nil {
		http.Error(w, "dashboard missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (a *App) apiInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, requestinfo.FromRequest(r, a.hostname, &a.material.Certificate))
}

func (a *App) apiResults(w http.ResponseWriter, _ *http.Request) {
	runs, err := a.log.ReadNewest(config.ResultsAPILimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (a *App) listBlobs(w http.ResponseWriter, _ *http.Request) {
	items, err := a.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blobs": items})
}

func (a *App) generateBlob(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	name := r.URL.Query().Get("name")
	size, err := strconv.ParseInt(r.URL.Query().Get("size"), 10, 64)
	if err != nil || size <= 0 {
		writeError(w, http.StatusBadRequest, "query size must be a positive integer")
		return
	}
	info, err := a.store.Generate(name, size)
	if err != nil {
		writeBlobError(w, err)
		return
	}
	a.record(r, results.Run{
		Operation: "generate",
		Name:      info.Name,
		Bytes:     info.Bytes,
		WriteMs:   info.WriteMs,
		TotalMs:   msSince(started),
	})
	writeJSON(w, http.StatusCreated, info)
}

func (a *App) uploadBlob(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	name := r.PathValue("name")
	var expected int64
	if r.ContentLength > 0 {
		expected = r.ContentLength
	}
	info, err := a.store.Put(name, r.Body, expected)
	if err != nil {
		writeBlobError(w, err)
		return
	}
	total := msSince(started)
	a.record(r, results.Run{
		Operation: "upload",
		Name:      info.Name,
		Bytes:     info.Bytes,
		WriteMs:   info.WriteMs,
		TotalMs:   total,
	})
	writeJSON(w, http.StatusOK, info)
}

func (a *App) downloadBlob(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	name := r.PathValue("name")
	file, info, err := a.store.Open(name)
	if err != nil {
		writeBlobError(w, err)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Bytes, 10))
	w.Header().Set("X-Blob-SHA256", info.SHA256)
	readStart := time.Now()
	_, copyErr := io.Copy(w, file)
	readMs := msSince(readStart)
	if copyErr != nil {
		log.Printf("download %s: %v", name, copyErr)
		return
	}
	a.record(r, results.Run{
		Operation: "download",
		Name:      info.Name,
		Bytes:     info.Bytes,
		ReadMs:    readMs,
		TotalMs:   msSince(started),
	})
}

func (a *App) deleteBlob(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Delete(r.PathValue("name")); err != nil {
		writeBlobError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) record(r *http.Request, run results.Run) {
	info := requestinfo.FromRequest(r, a.hostname, &a.material.Certificate)
	run.TLS = info.TLS
	run.ClientAddr = info.ClientAddr
	run.TLSVersion = info.TLSVersion
	run.Cipher = info.Cipher
	if err := a.log.Append(run); err != nil {
		log.Printf("results log: %v", err)
	}
}

func writeBlobError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, blobs.ErrInvalidName), errors.Is(err, blobs.ErrLengthMismatch), errors.Is(err, blobs.ErrSizeMismatch):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, blobs.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, blobs.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := encodeJSON(w, payload); err != nil {
		log.Printf("write json: %v", err)
	}
}

func msSince(started time.Time) float64 {
	return float64(time.Since(started).Microseconds()) / 1000.0
}

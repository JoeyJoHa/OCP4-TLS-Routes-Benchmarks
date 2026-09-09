package results

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/timing"
)

const (
	dirPerm     = 0o750
	filePerm    = 0o644
	bytesPerMiB = 1024 * 1024
)

// ErrNoMatchingRun is returned when client timings cannot be attached.
var ErrNoMatchingRun = errors.New("no matching run")

// Run is one timed generate, upload, or download.
type Run struct {
	Timestamp            time.Time `json:"timestamp"`
	Operation            string    `json:"operation"`
	Name                 string    `json:"name"`
	Bytes                int64     `json:"bytes"`
	TLS                  bool      `json:"tls"`
	ClientAddr           string    `json:"client_addr"`
	WriteMs              float64   `json:"write_ms,omitempty"`
	ReadMs               float64   `json:"read_ms,omitempty"`
	TotalMs              float64   `json:"total_ms"`
	ThroughputMiBs       float64   `json:"throughput_mib_s"`
	TLSVersion           string    `json:"tls_version,omitempty"`
	Cipher               string    `json:"cipher,omitempty"`
	TLSKeyAlgorithm      string    `json:"tls_key_algorithm,omitempty"`
	TLSKeySize           int       `json:"tls_key_size,omitempty"`
	TLSKeyCurve          string    `json:"tls_key_curve,omitempty"`
	DNSMs                float64   `json:"dns_ms,omitempty"`
	TCPConnectMs         float64   `json:"tcp_connect_ms,omitempty"`
	TLSHandshakeMs       float64   `json:"tls_handshake_ms,omitempty"`
	TLSHandshakeServerMs float64   `json:"tls_handshake_server_ms,omitempty"`
	TLSReused            bool      `json:"tls_reused,omitempty"`
	RedirectMs           float64   `json:"redirect_ms,omitempty"`
	TTFBMs               float64   `json:"ttfb_ms,omitempty"`
	TransferMs           float64   `json:"transfer_ms,omitempty"`
	ClientTotalMs        float64   `json:"client_total_ms,omitempty"`
}

// Logger appends JSON lines to RESULTS_LOG on the PVC.
type Logger struct {
	mu   sync.Mutex
	path string
}

func NewLogger(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return nil, err
	}
	return &Logger{path: path}, nil
}

func (l *Logger) Append(run Run) error {
	if run.Timestamp.IsZero() {
		run.Timestamp = time.Now().UTC()
	}
	run.ThroughputMiBs = throughput(run.Bytes, run.TotalMs)
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.appendUnlocked(run)
}

// MergeClientTimings attaches curl phase timings to the newest matching run.
func (l *Logger) MergeClientTimings(name, operation string, phases timing.Phases) (Run, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	runs, err := l.readAllUnlocked()
	if err != nil {
		return Run{}, err
	}
	idx := -1
	for i, run := range runs {
		if run.Name == name && run.Operation == operation {
			idx = i
		}
	}
	if idx < 0 {
		return Run{}, ErrNoMatchingRun
	}
	runs[idx].DNSMs = phases.DNSMs
	runs[idx].TCPConnectMs = phases.TCPConnectMs
	runs[idx].RedirectMs = phases.RedirectMs
	runs[idx].TTFBMs = phases.TTFBMs
	runs[idx].TransferMs = phases.TransferMs
	runs[idx].ClientTotalMs = phases.ClientTotalMs
	if phases.TLSHandshakeMs > 0 {
		runs[idx].TLSHandshakeMs = phases.TLSHandshakeMs
		runs[idx].TLSReused = false
	}
	if err := l.rewriteUnlocked(runs); err != nil {
		return Run{}, err
	}
	return runs[idx], nil
}

// ReadNewest returns up to limit runs, newest first.
func (l *Logger) ReadNewest(limit int) ([]Run, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	runs, err := l.readAllUnlocked()
	if err != nil {
		return nil, err
	}
	reverse(runs)
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

func (l *Logger) appendUnlocked(run Run) error {
	line, err := json.Marshal(run)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, filePerm)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (l *Logger) readAllUnlocked() ([]Run, error) {
	file, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Run{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var runs []Run
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var run Run
		if err := json.Unmarshal(line, &run); err != nil {
			continue
		}
		runs = append(runs, run)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func (l *Logger) rewriteUnlocked(runs []Run) error {
	tmp := l.path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, filePerm)
	if err != nil {
		return err
	}
	for _, run := range runs {
		line, err := json.Marshal(run)
		if err != nil {
			file.Close()
			_ = os.Remove(tmp)
			return err
		}
		if _, err := file.Write(append(line, '\n')); err != nil {
			file.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, l.path)
}

func throughput(bytes int64, totalMs float64) float64 {
	if totalMs <= 0 || bytes <= 0 {
		return 0
	}
	seconds := totalMs / 1000.0
	return float64(bytes) / bytesPerMiB / seconds
}

func reverse(runs []Run) {
	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
		runs[i], runs[j] = runs[j], runs[i]
	}
}

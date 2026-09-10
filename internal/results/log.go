package results

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/timing"
)

const (
	dirPerm          = 0o750
	filePerm         = 0o644
	bytesPerMiB      = 1024 * 1024
	scannerBufSize   = 64 * 1024
	scannerMaxTokens = 1024 * 1024
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
	ALPN                 string    `json:"alpn,omitempty"`
	RouteMode            string    `json:"route_mode,omitempty"`
	ExperimentID         string    `json:"experiment_id,omitempty"`
	SampleIndex          int       `json:"sample_index,omitempty"`
}

// ClientTimingMeta is optional metadata posted with curl phase timings.
type ClientTimingMeta struct {
	RouteMode    string
	ExperimentID string
	SampleIndex  int
}

// Logger appends JSON lines to RESULTS_LOG on the PVC.
type Logger struct {
	mu   sync.Mutex
	path string
}

func NewLogger(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return nil, fmt.Errorf("create results directory: %w", err)
	}
	return &Logger{path: path}, nil
}

func (l *Logger) Append(run Run) error {
	if run.Timestamp.IsZero() {
		run.Timestamp = time.Now().UTC()
	}
	run.ThroughputMiBs = throughput(run)
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.appendUnlocked(run)
}

// MergeClientTimings attaches curl phase timings to the newest matching run.
func (l *Logger) MergeClientTimings(name, operation string, phases timing.Phases, meta ClientTimingMeta) (Run, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	runs, err := l.readAllUnlocked()
	if err != nil {
		return Run{}, fmt.Errorf("read results log: %w", err)
	}
	idx := findMergeIndex(runs, name, operation, meta)
	if idx < 0 {
		return Run{}, ErrNoMatchingRun
	}
	applyClientPhases(&runs[idx], phases)
	applyClientMeta(&runs[idx], meta)
	runs[idx].ThroughputMiBs = throughput(runs[idx])
	if err := l.rewriteUnlocked(runs); err != nil {
		return Run{}, fmt.Errorf("rewrite results log: %w", err)
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
		return fmt.Errorf("marshal run: %w", err)
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, filePerm)
	if err != nil {
		return fmt.Errorf("open results log: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append results log: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync results log: %w", err)
	}
	return nil
}

func (l *Logger) readAllUnlocked() ([]Run, error) {
	file, err := os.Open(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Run{}, nil
		}
		return nil, fmt.Errorf("open results log: %w", err)
	}
	defer file.Close()

	var runs []Run
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, scannerBufSize), scannerMaxTokens)
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
		return nil, fmt.Errorf("scan results log: %w", err)
	}
	return runs, nil
}

func (l *Logger) rewriteUnlocked(runs []Run) (err error) {
	tmp := l.path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, filePerm)
	if err != nil {
		return fmt.Errorf("create results temp file: %w", err)
	}
	defer func() {
		_ = file.Close()
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	for _, run := range runs {
		line, marshalErr := json.Marshal(run)
		if marshalErr != nil {
			return fmt.Errorf("marshal run: %w", marshalErr)
		}
		if _, writeErr := file.Write(append(line, '\n')); writeErr != nil {
			return fmt.Errorf("write results temp file: %w", writeErr)
		}
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync results temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close results temp file: %w", err)
	}
	if err := os.Rename(tmp, l.path); err != nil {
		return fmt.Errorf("replace results log: %w", err)
	}
	return nil
}

func throughput(run Run) float64 {
	if run.Bytes <= 0 {
		return 0
	}
	durationMs := bulkDurationMs(run)
	if durationMs <= 0 {
		return 0
	}
	seconds := durationMs / 1000.0
	return float64(run.Bytes) / bytesPerMiB / seconds
}

// bulkDurationMs picks the client interval that actually moved the blob bytes.
// curl transfer_ms (total − starttransfer) is the download response body; on PUT
// uploads the payload is sent before starttransfer, so use ttfb_ms instead.
func bulkDurationMs(run Run) float64 {
	switch run.Operation {
	case "upload":
		if run.TTFBMs > 0 {
			return run.TTFBMs
		}
	case "download":
		if run.TransferMs > 0 {
			return run.TransferMs
		}
	default:
		if run.TransferMs > 0 {
			return run.TransferMs
		}
	}
	if run.ClientTotalMs > 0 {
		return run.ClientTotalMs
	}
	return run.TotalMs
}

func findMergeIndex(runs []Run, name, operation string, meta ClientTimingMeta) int {
	idx := -1
	for i, run := range runs {
		if run.Operation != operation {
			continue
		}
		if meta.ExperimentID != "" {
			if run.ExperimentID != meta.ExperimentID || run.SampleIndex != meta.SampleIndex {
				continue
			}
			idx = i
			continue
		}
		if run.Name == name {
			idx = i
		}
	}
	return idx
}

func applyClientPhases(run *Run, phases timing.Phases) {
	run.DNSMs = phases.DNSMs
	run.TCPConnectMs = phases.TCPConnectMs
	run.RedirectMs = phases.RedirectMs
	run.TTFBMs = phases.TTFBMs
	run.TransferMs = phases.TransferMs
	run.ClientTotalMs = phases.ClientTotalMs
	if phases.TLSHandshakeMs > 0 {
		run.TLSHandshakeMs = phases.TLSHandshakeMs
	}
}

func applyClientMeta(run *Run, meta ClientTimingMeta) {
	if meta.RouteMode != "" {
		run.RouteMode = meta.RouteMode
	}
	if meta.ExperimentID != "" {
		run.ExperimentID = meta.ExperimentID
	}
	if meta.SampleIndex > 0 {
		run.SampleIndex = meta.SampleIndex
	}
}

func reverse(runs []Run) {
	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
		runs[i], runs[j] = runs[j], runs[i]
	}
}

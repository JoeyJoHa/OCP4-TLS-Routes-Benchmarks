package results

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	dirPerm     = 0o750
	filePerm    = 0o644
	bytesPerMiB = 1024 * 1024
)

// Run is one timed generate, upload, or download.
type Run struct {
	Timestamp      time.Time `json:"timestamp"`
	Operation      string    `json:"operation"`
	Name           string    `json:"name"`
	Bytes          int64     `json:"bytes"`
	TLS            bool      `json:"tls"`
	ClientAddr     string    `json:"client_addr"`
	WriteMs        float64   `json:"write_ms,omitempty"`
	ReadMs         float64   `json:"read_ms,omitempty"`
	TotalMs        float64   `json:"total_ms"`
	ThroughputMiBs float64   `json:"throughput_mib_s"`
	TLSVersion     string    `json:"tls_version,omitempty"`
	Cipher         string    `json:"cipher,omitempty"`
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
	line, err := json.Marshal(run)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

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

// ReadNewest returns up to limit runs, newest first.
func (l *Logger) ReadNewest(limit int) ([]Run, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

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
	reverse(runs)
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
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

package results

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndReadNewest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	logger, err := NewLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		err := logger.Append(Run{
			Timestamp:  time.Unix(int64(i+1), 0).UTC(),
			Operation:  "upload",
			Name:       "blob.bin",
			Bytes:      1024 * 1024,
			TotalMs:    1000,
			ClientAddr: "10.0.0.8",
			TLS:        true,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	runs, err := logger.ReadNewest(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("len=%d", len(runs))
	}
	if runs[0].Timestamp.Before(runs[1].Timestamp) {
		t.Fatal("expected newest first")
	}
	if runs[0].ThroughputMiBs <= 0 {
		t.Fatalf("throughput=%v", runs[0].ThroughputMiBs)
	}
}

func TestReadMissingLog(t *testing.T) {
	logger, err := NewLogger(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	runs, err := logger.ReadNewest(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("len=%d", len(runs))
	}
}

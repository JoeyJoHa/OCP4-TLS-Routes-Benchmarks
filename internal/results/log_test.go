package results

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/timing"
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

func TestMergeClientTimingsUpdatesNewestMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	logger, err := NewLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(Run{Operation: "upload", Name: "a.bin", Bytes: 8, TotalMs: 10}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(Run{Operation: "upload", Name: "a.bin", Bytes: 8, TotalMs: 20, TLSHandshakeServerMs: 12}); err != nil {
		t.Fatal(err)
	}
	updated, err := logger.MergeClientTimings("a.bin", "upload", timing.Phases{
		DNSMs:          1.5,
		TCPConnectMs:   2.5,
		TLSHandshakeMs: 40,
		TTFBMs:         5,
		TransferMs:     80,
		ClientTotalMs:  90,
	}, ClientTimingMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.TotalMs != 20 {
		t.Fatalf("should merge into newest run, total_ms=%v", updated.TotalMs)
	}
	if updated.DNSMs != 1.5 || updated.TLSHandshakeMs != 40 || updated.TLSHandshakeServerMs != 12 {
		t.Fatalf("merged=%+v", updated)
	}
	runs, err := logger.ReadNewest(10)
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].DNSMs != 1.5 || runs[1].DNSMs != 0 {
		t.Fatalf("only newest row should have client timings: %+v", runs)
	}
}

func TestMergeClientTimingsMissingRun(t *testing.T) {
	logger, err := NewLogger(filepath.Join(t.TempDir(), "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = logger.MergeClientTimings("missing.bin", "upload", timing.Phases{}, ClientTimingMeta{})
	if !errors.Is(err, ErrNoMatchingRun) {
		t.Fatalf("err=%v", err)
	}
}

func TestMergeClientTimingsPreservesServerTLSReused(t *testing.T) {
	logger, err := NewLogger(filepath.Join(t.TempDir(), "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(Run{
		Operation:            "upload",
		Name:                 "reuse.bin",
		Bytes:                8,
		TotalMs:              12,
		TLSReused:            true,
		TLSHandshakeServerMs: 0,
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := logger.MergeClientTimings("reuse.bin", "upload", timing.Phases{
		TLSHandshakeMs: 4.2,
		ClientTotalMs:  15,
	}, ClientTimingMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.TLSReused {
		t.Fatalf("client handshake ms must not clear server TLSReused: %+v", updated)
	}
	if updated.TLSHandshakeMs != 4.2 {
		t.Fatalf("expected client handshake ms, got %+v", updated)
	}
}

func TestMergeClientTimingsDoesNotInventReuseFromHandshakeMs(t *testing.T) {
	logger, err := NewLogger(filepath.Join(t.TempDir(), "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(Run{
		Operation: "handshake",
		Name:      "cold.bin",
		TotalMs:   8,
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := logger.MergeClientTimings("cold.bin", "handshake", timing.Phases{
		TLSHandshakeMs: 12,
		ClientTotalMs:  14,
	}, ClientTimingMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.TLSReused {
		t.Fatalf("cold handshake must not set TLSReused: %+v", updated)
	}
}

func TestMergeClientTimingsByExperimentID(t *testing.T) {
	logger, err := NewLogger(filepath.Join(t.TempDir(), "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(Run{
		Operation:            "handshake",
		Name:                 "exp1-2",
		ExperimentID:         "exp1",
		SampleIndex:          2,
		TLSHandshakeServerMs: 11,
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := logger.MergeClientTimings("", "handshake", timing.Phases{
		TLSHandshakeMs: 22,
		ClientTotalMs:  25,
	}, ClientTimingMeta{ExperimentID: "exp1", SampleIndex: 2, RouteMode: "passthrough"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.TLSHandshakeMs != 22 || updated.TLSHandshakeServerMs != 11 || updated.RouteMode != "passthrough" {
		t.Fatalf("updated=%+v", updated)
	}
}

func TestThroughputDownloadUsesTransferMs(t *testing.T) {
	run := Run{
		Operation:  "download",
		Bytes:      1048576,
		TotalMs:    1000,
		TransferMs: 100,
	}
	got := throughput(run)
	want := float64(1048576) / bytesPerMiB / 0.1
	if got != want {
		t.Fatalf("throughput=%v want %v", got, want)
	}
}

func TestThroughputUploadUsesTTFBNotResponseBody(t *testing.T) {
	run := Run{
		Operation:     "upload",
		Bytes:         1048576,
		TotalMs:       1000,
		TTFBMs:        50,
		TransferMs:    0.2,
		ClientTotalMs: 63,
	}
	got := throughput(run)
	want := float64(1048576) / bytesPerMiB / 0.05
	if got != want {
		t.Fatalf("throughput=%v want %v", got, want)
	}
	if got > 1000 {
		t.Fatalf("upload throughput inflated by response body transfer_ms: %v", got)
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

package config

import (
	"os"
	"testing"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv(EnvHTTPAddr, "")
	t.Setenv(EnvMaxBlobBytes, "")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != DefaultHTTPAddr {
		t.Fatalf("HTTPAddr=%q", cfg.HTTPAddr)
	}
	if cfg.MaxBlobBytes != DefaultMaxBlobBytes {
		t.Fatalf("MaxBlobBytes=%d", cfg.MaxBlobBytes)
	}
	if len(cfg.TLSDNSNames) == 0 || cfg.TLSDNSNames[0] != DefaultDNSNames {
		t.Fatalf("TLSDNSNames=%v", cfg.TLSDNSNames)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv(EnvHTTPAddr, ":9090")
	t.Setenv(EnvDataDir, "/tmp/tlsbench")
	t.Setenv(EnvMaxBlobBytes, "1048576")
	t.Setenv(EnvTLSDNSNames, "app.example.com, localhost, app.example.com")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("HTTPAddr=%q", cfg.HTTPAddr)
	}
	if cfg.DataDir != "/tmp/tlsbench" {
		t.Fatalf("DataDir=%q", cfg.DataDir)
	}
	if cfg.MaxBlobBytes != 1048576 {
		t.Fatalf("MaxBlobBytes=%d", cfg.MaxBlobBytes)
	}
	if len(cfg.TLSDNSNames) != 2 {
		t.Fatalf("expected deduped SANs, got %v", cfg.TLSDNSNames)
	}
}

func TestFromEnvRejectsInvalidMax(t *testing.T) {
	t.Setenv(EnvMaxBlobBytes, "nope")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error")
	}
	t.Setenv(EnvMaxBlobBytes, "0")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error for zero")
	}
	_ = os.Unsetenv(EnvMaxBlobBytes)
}

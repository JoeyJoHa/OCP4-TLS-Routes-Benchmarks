package certs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
)

func TestLoadOrGenerateWritesFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		TLSCertFile: filepath.Join(dir, "tls.crt"),
		TLSKeyFile:  filepath.Join(dir, "tls.key"),
		TLSCAFile:   filepath.Join(dir, "ca.crt"),
		TLSDNSNames: []string{"tlsbench.example.com", "127.0.0.1"},
	}
	material, err := LoadOrGenerate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !material.Generated {
		t.Fatal("expected generated material")
	}
	if material.Certificate.Leaf == nil {
		t.Fatal("expected parsed leaf")
	}
	if material.Certificate.Leaf.Subject.CommonName != "tlsbench" {
		t.Fatalf("CN=%s", material.Certificate.Leaf.Subject.CommonName)
	}
	foundSAN := false
	for _, name := range material.Certificate.Leaf.DNSNames {
		if name == "tlsbench.example.com" {
			foundSAN = true
		}
	}
	if !foundSAN {
		t.Fatalf("missing SAN: %v", material.Certificate.Leaf.DNSNames)
	}
	if _, err := os.Stat(cfg.TLSCAFile); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadOrGenerate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Generated {
		t.Fatal("second load should use existing files")
	}
}

func TestLoadOrGenerateRejectsPartialFiles(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	if err := os.WriteFile(certPath, []byte("not-a-cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadOrGenerate(config.Config{
		TLSCertFile: certPath,
		TLSKeyFile:  filepath.Join(dir, "tls.key"),
		TLSCAFile:   filepath.Join(dir, "ca.crt"),
	})
	if err == nil {
		t.Fatal("expected error when only cert exists")
	}
}

func TestLoadExtraCAs(t *testing.T) {
	dir := t.TempDir()
	ca := pemCA(t, dir)
	pool, err := LoadExtraCAs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pool == nil {
		t.Fatal("expected pool")
	}
	_ = ca
	missing, err := LoadExtraCAs(filepath.Join(dir, "nope"))
	if err != nil || missing != nil {
		t.Fatalf("missing dir should be ignored: %v %v", missing, err)
	}
}

func pemCA(t *testing.T, dir string) string {
	t.Helper()
	cfg := config.Config{
		TLSCertFile: filepath.Join(dir, "tls.crt"),
		TLSKeyFile:  filepath.Join(dir, "tls.key"),
		TLSCAFile:   filepath.Join(dir, "internal.pem"),
		TLSDNSNames: []string{"localhost"},
	}
	if _, err := LoadOrGenerate(cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.TLSCAFile
}

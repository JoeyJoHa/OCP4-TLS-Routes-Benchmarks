package certs

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
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
	if _, err := os.Stat(caKeyPath(cfg.TLSCAFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("generated CA private key must not be written: %v", err)
	}
	if material.Certificate.Leaf.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
		t.Fatal("ECDSA server cert must not include KeyEncipherment")
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

func TestLoadOrGenerateReportsStatError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadOrGenerate(config.Config{
		TLSCertFile: filepath.Join(blocker, "tls.crt"),
		TLSKeyFile:  filepath.Join(dir, "tls.key"),
		TLSCAFile:   filepath.Join(dir, "ca.crt"),
	})
	if err == nil {
		t.Fatal("expected stat error when cert path parent is a file")
	}
}

func TestServerTLSConfigPinsMinVersionAndCiphers(t *testing.T) {
	dir := t.TempDir()
	material, err := LoadOrGenerate(config.Config{
		TLSCertFile: filepath.Join(dir, "tls.crt"),
		TLSKeyFile:  filepath.Join(dir, "tls.key"),
		TLSCAFile:   filepath.Join(dir, "ca.crt"),
		TLSDNSNames: []string{"localhost"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := ServerTLSConfig(material, nil, config.Config{
		TLSMinVersion:         tls.VersionTLS13,
		TLSCipherSuites:       []uint16{tls.TLS_AES_128_GCM_SHA256},
		DisableSessionTickets: true,
	})
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("MinVersion=%d", cfg.MinVersion)
	}
	if !cfg.SessionTicketsDisabled {
		t.Fatal("expected SessionTicketsDisabled")
	}
	if len(cfg.CipherSuites) != 1 || cfg.CipherSuites[0] != tls.TLS_AES_128_GCM_SHA256 {
		t.Fatalf("CipherSuites=%v", cfg.CipherSuites)
	}
	if cfg.NextProtos[0] != "h2" {
		t.Fatalf("NextProtos=%v", cfg.NextProtos)
	}
}

func TestLoadExtraCAs(t *testing.T) {
	dir := t.TempDir()
	pemCA(t, dir)
	pool, err := LoadExtraCAs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pool == nil {
		t.Fatal("expected pool")
	}
	missing, err := LoadExtraCAs(filepath.Join(dir, "nope"))
	if err != nil || missing != nil {
		t.Fatalf("missing dir should be ignored: %v %v", missing, err)
	}
}

func TestCAKeyPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{in: "/certs/ca.crt", want: "/certs/ca.key"},
		{in: "/certs/ca", want: "/certs/ca.key"},
		{in: "ca.pem", want: "ca.key"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := caKeyPath(tt.in); got != tt.want {
				t.Fatalf("caKeyPath(%q)=%q want %q", tt.in, got, tt.want)
			}
		})
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

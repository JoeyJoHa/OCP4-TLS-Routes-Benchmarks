package certs

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
)

// LoadClientCAs returns a pool when mTLS is enabled via TLS_CLIENT_CA_FILE.
func LoadClientCAs(path string) (*x509.CertPool, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read TLS_CLIENT_CA_FILE: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("TLS_CLIENT_CA_FILE contains no certificates")
	}
	return pool, nil
}

// LoadExtraCAs reads PEM files from TLS_CA_DIR. The HTTPS server does not use
// this pool; scripts/entrypoint.sh merges the same directory into CURL_CA_BUNDLE.
func LoadExtraCAs(dir string) (*x509.CertPool, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read TLS_CA_DIR: %w", err)
	}
	pool := x509.NewCertPool()
	found := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".crt" && ext != ".pem" && ext != ".cer" {
			continue
		}
		pemBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read extra CA %s: %w", name, err)
		}
		if pool.AppendCertsFromPEM(pemBytes) {
			found = true
		}
	}
	if !found {
		return nil, nil
	}
	return pool, nil
}

// ServerTLSConfig builds the HTTPS listener configuration.
func ServerTLSConfig(material Material, clientCAs *x509.CertPool, cfg config.Config) *tls.Config {
	minVersion := cfg.TLSMinVersion
	if minVersion == 0 {
		minVersion = tls.VersionTLS12
	}
	tlsCfg := &tls.Config{
		MinVersion:             minVersion,
		Certificates:           []tls.Certificate{material.Certificate},
		NextProtos:             append([]string(nil), config.DefaultTLSNextProtos...),
		SessionTicketsDisabled: cfg.DisableSessionTickets,
	}
	if len(cfg.TLSCipherSuites) > 0 {
		tlsCfg.CipherSuites = append([]uint16(nil), cfg.TLSCipherSuites...)
	}
	if clientCAs != nil {
		tlsCfg.ClientCAs = clientCAs
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return tlsCfg
}

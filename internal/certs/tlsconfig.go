package certs

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// LoadExtraCAs reads PEM files from TLS_CA_DIR for pod-to-pod trust.
func LoadExtraCAs(dir string) (*x509.CertPool, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
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
			return nil, err
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
func ServerTLSConfig(material Material, clientCAs *x509.CertPool) *tls.Config {
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{material.Certificate},
		NextProtos:   []string{"h2", "http/1.1"},
	}
	if clientCAs != nil {
		cfg.ClientCAs = clientCAs
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg
}

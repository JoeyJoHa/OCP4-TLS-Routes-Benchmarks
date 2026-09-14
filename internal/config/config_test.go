package config

import (
	"crypto/tls"
	"testing"
)

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
		check   func(*testing.T, Config)
	}{
		{
			name: "defaults",
			env:  map[string]string{EnvHTTPAddr: "", EnvMaxBlobBytes: ""},
			check: func(t *testing.T, cfg Config) {
				if cfg.HTTPAddr != DefaultHTTPAddr {
					t.Fatalf("HTTPAddr=%q", cfg.HTTPAddr)
				}
				if cfg.MaxBlobBytes != DefaultMaxBlobBytes {
					t.Fatalf("MaxBlobBytes=%d", cfg.MaxBlobBytes)
				}
				if len(cfg.TLSDNSNames) == 0 || cfg.TLSDNSNames[0] != DefaultDNSNames {
					t.Fatalf("TLSDNSNames=%v", cfg.TLSDNSNames)
				}
			},
		},
		{
			name: "overrides and SAN dedupe",
			env: map[string]string{
				EnvHTTPAddr:     ":9090",
				EnvDataDir:      "/tmp/tlsbench",
				EnvMaxBlobBytes: "1048576",
				EnvTLSDNSNames:  "app.example.com, localhost, app.example.com",
			},
			check: func(t *testing.T, cfg Config) {
				if cfg.HTTPAddr != ":9090" || cfg.DataDir != "/tmp/tlsbench" || cfg.MaxBlobBytes != 1048576 {
					t.Fatalf("cfg=%+v", cfg)
				}
				if len(cfg.TLSDNSNames) != 2 {
					t.Fatalf("expected deduped SANs, got %v", cfg.TLSDNSNames)
				}
			},
		},
		{
			name:    "invalid max",
			env:     map[string]string{EnvMaxBlobBytes: "nope"},
			wantErr: true,
		},
		{
			name: "tls min version 1.3 and tickets off by default",
			env:  map[string]string{EnvTLSMinVersion: "1.3"},
			check: func(t *testing.T, cfg Config) {
				if cfg.TLSMinVersion != tls.VersionTLS13 {
					t.Fatalf("TLSMinVersion=%d", cfg.TLSMinVersion)
				}
				if !cfg.DisableSessionTickets {
					t.Fatal("session tickets should be disabled by default")
				}
				if !cfg.ServeCA {
					t.Fatal("SERVE_CA should default true")
				}
			},
		},
		{
			name:    "bad min version",
			env:     map[string]string{EnvTLSMinVersion: "1.1"},
			wantErr: true,
		},
		{
			name: "serve ca off and tickets on",
			env: map[string]string{
				EnvServeCA:           "false",
				EnvTLSDisableTickets: "false",
				EnvWriteToken:        "lab-token",
			},
			check: func(t *testing.T, cfg Config) {
				if cfg.ServeCA {
					t.Fatal("ServeCA should be false")
				}
				if cfg.DisableSessionTickets {
					t.Fatal("session tickets should stay enabled when env is false")
				}
				if cfg.WriteToken != "lab-token" {
					t.Fatalf("WriteToken=%q", cfg.WriteToken)
				}
			},
		},
		{
			name: "cipher suite pin",
			env:  map[string]string{EnvTLSCipherSuites: "TLS_AES_128_GCM_SHA256"},
			check: func(t *testing.T, cfg Config) {
				if len(cfg.TLSCipherSuites) != 1 || cfg.TLSCipherSuites[0] != tls.TLS_AES_128_GCM_SHA256 {
					t.Fatalf("TLSCipherSuites=%v", cfg.TLSCipherSuites)
				}
			},
		},
		{
			name:    "unknown cipher",
			env:     map[string]string{EnvTLSCipherSuites: "RC4-MD5"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvHTTPAddr, "")
			t.Setenv(EnvDataDir, "")
			t.Setenv(EnvMaxBlobBytes, "")
			t.Setenv(EnvTLSDNSNames, "")
			t.Setenv(EnvTLSMinVersion, "")
			t.Setenv(EnvTLSCipherSuites, "")
			t.Setenv(EnvTLSDisableTickets, "")
			t.Setenv(EnvServeCA, "")
			t.Setenv(EnvWriteToken, "")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			cfg, err := FromEnv()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, cfg)
		})
	}
}

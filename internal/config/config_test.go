package config

import "testing"

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
			name:    "zero max",
			env:     map[string]string{EnvMaxBlobBytes: "0"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvHTTPAddr, "")
			t.Setenv(EnvDataDir, "")
			t.Setenv(EnvMaxBlobBytes, "")
			t.Setenv(EnvTLSDNSNames, "")
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

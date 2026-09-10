package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultHTTPAddr       = ":8080"
	DefaultHTTPSAddr      = ":8443"
	DefaultTLSCertFile    = "/certs/tls.crt"
	DefaultTLSKeyFile     = "/certs/tls.key"
	DefaultTLSCAFile      = "/certs/ca.crt"
	DefaultTLSCADir       = "/etc/pki/internal-ca"
	DefaultTLSCABundle    = "/certs/ca-bundle.pem"
	DefaultDataDir        = "/data"
	DefaultMaxBlobBytes   = 256 * 1024 * 1024
	DefaultResultsLog     = "/data/results/runs.jsonl"
	DefaultDNSNames       = "localhost"
	ReadHeaderTimeoutSecs = 10
	IdleTimeoutSecs       = 120
	ShutdownTimeoutSecs   = 10
	ResultsAPILimit       = 500
	MaxBlobNameLength     = 128
	MaxExperimentIDLength = 128
	MaxJSONBodyBytes      = 32 * 1024
	MaxHeaderBytes        = 1 << 20
	CertValidityDays      = 365
)

const (
	EnvHTTPAddr        = "HTTP_ADDR"
	EnvHTTPSAddr       = "HTTPS_ADDR"
	EnvTLSCertFile     = "TLS_CERT_FILE"
	EnvTLSKeyFile      = "TLS_KEY_FILE"
	EnvTLSCAFile       = "TLS_CA_FILE"
	EnvTLSCADir        = "TLS_CA_DIR"
	EnvTLSCABundleFile = "TLS_CA_BUNDLE_FILE"
	EnvTLSClientCAFile = "TLS_CLIENT_CA_FILE"
	EnvTLSDNSNames     = "TLS_DNS_NAMES"
	EnvDataDir         = "DATA_DIR"
	EnvMaxBlobBytes    = "MAX_BLOB_BYTES"
	EnvResultsLog      = "RESULTS_LOG"
)

// DefaultTLSNextProtos is the ALPN list advertised on HTTPS (HTTP/2 preferred).
var DefaultTLSNextProtos = []string{"h2", "http/1.1"}

// Config is runtime configuration loaded from environment variables.
type Config struct {
	HTTPAddr        string
	HTTPSAddr       string
	TLSCertFile     string
	TLSKeyFile      string
	TLSCAFile       string
	TLSCADir        string
	TLSCABundleFile string
	TLSClientCAFile string
	TLSDNSNames     []string
	DataDir         string
	MaxBlobBytes    int64
	ResultsLog      string
}

// FromEnv loads configuration, applying defaults when a variable is unset.
func FromEnv() (Config, error) {
	maxBytes, err := intFromEnv(EnvMaxBlobBytes, DefaultMaxBlobBytes)
	if err != nil {
		return Config{}, err
	}
	if maxBytes <= 0 {
		return Config{}, fmt.Errorf("%s must be greater than 0", EnvMaxBlobBytes)
	}

	cfg := Config{
		HTTPAddr:        stringFromEnv(EnvHTTPAddr, DefaultHTTPAddr),
		HTTPSAddr:       stringFromEnv(EnvHTTPSAddr, DefaultHTTPSAddr),
		TLSCertFile:     stringFromEnv(EnvTLSCertFile, DefaultTLSCertFile),
		TLSKeyFile:      stringFromEnv(EnvTLSKeyFile, DefaultTLSKeyFile),
		TLSCAFile:       stringFromEnv(EnvTLSCAFile, DefaultTLSCAFile),
		TLSCADir:        stringFromEnv(EnvTLSCADir, DefaultTLSCADir),
		TLSCABundleFile: stringFromEnv(EnvTLSCABundleFile, DefaultTLSCABundle),
		TLSClientCAFile: strings.TrimSpace(os.Getenv(EnvTLSClientCAFile)),
		TLSDNSNames:     splitCSV(stringFromEnv(EnvTLSDNSNames, DefaultDNSNames)),
		DataDir:         stringFromEnv(EnvDataDir, DefaultDataDir),
		MaxBlobBytes:    maxBytes,
		ResultsLog:      stringFromEnv(EnvResultsLog, DefaultResultsLog),
	}
	return cfg, nil
}

func stringFromEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func intFromEnv(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	names := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if len(names) == 0 {
		return []string{DefaultDNSNames}
	}
	return names
}

package config

import (
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultHTTPAddr        = ":8080"
	DefaultHTTPSAddr       = ":8443"
	DefaultTLSCertFile     = "/certs/tls.crt"
	DefaultTLSKeyFile      = "/certs/tls.key"
	DefaultTLSCAFile       = "/certs/ca.crt"
	DefaultTLSCADir        = "/etc/pki/internal-ca"
	DefaultTLSCABundle     = "/certs/ca-bundle.pem"
	DefaultDataDir         = "/data"
	DefaultMaxBlobBytes    = 256 * 1024 * 1024
	DefaultResultsLog      = "/data/results/runs.jsonl"
	DefaultDNSNames        = "localhost"
	ReadHeaderTimeoutSecs  = 10
	ReadTimeoutSecs        = 300
	WriteTimeoutSecs       = 300
	IdleTimeoutSecs        = 120
	ShutdownTimeoutSecs    = 10
	ResultsAPIDefaultLimit = 2000
	ResultsAPIMaxLimit     = 10000
	MaxBlobNameLength      = 128
	MaxExperimentIDLength  = 128
	MaxJSONBodyBytes       = 32 * 1024
	MaxHeaderBytes         = 1 << 20
	CertValidityDays       = 365
)

const (
	EnvHTTPAddr          = "HTTP_ADDR"
	EnvHTTPSAddr         = "HTTPS_ADDR"
	EnvTLSCertFile       = "TLS_CERT_FILE"
	EnvTLSKeyFile        = "TLS_KEY_FILE"
	EnvTLSCAFile         = "TLS_CA_FILE"
	EnvTLSCADir          = "TLS_CA_DIR"
	EnvTLSCABundleFile   = "TLS_CA_BUNDLE_FILE"
	EnvTLSClientCAFile   = "TLS_CLIENT_CA_FILE"
	EnvTLSDNSNames       = "TLS_DNS_NAMES"
	EnvDataDir           = "DATA_DIR"
	EnvMaxBlobBytes      = "MAX_BLOB_BYTES"
	EnvResultsLog        = "RESULTS_LOG"
	EnvWriteToken        = "BENCH_WRITE_TOKEN"
	EnvServeCA           = "SERVE_CA"
	EnvTLSMinVersion     = "TLS_MIN_VERSION"
	EnvTLSCipherSuites   = "TLS_CIPHER_SUITES"
	EnvTLSDisableTickets = "TLS_DISABLE_SESSION_TICKETS"
)

// DefaultTLSNextProtos is the ALPN list advertised on HTTPS (HTTP/2 preferred).
var DefaultTLSNextProtos = []string{"h2", "http/1.1"}

// Config is runtime configuration loaded from environment variables.
type Config struct {
	HTTPAddr              string
	HTTPSAddr             string
	TLSCertFile           string
	TLSKeyFile            string
	TLSCAFile             string
	TLSCADir              string
	TLSCABundleFile       string
	TLSClientCAFile       string
	TLSDNSNames           []string
	DataDir               string
	MaxBlobBytes          int64
	ResultsLog            string
	WriteToken            string
	ServeCA               bool
	TLSMinVersion         uint16
	TLSCipherSuites       []uint16
	DisableSessionTickets bool
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

	serveCA, err := boolFromEnv(EnvServeCA, true)
	if err != nil {
		return Config{}, err
	}
	disableTickets, err := boolFromEnv(EnvTLSDisableTickets, true)
	if err != nil {
		return Config{}, err
	}
	minVersion, err := tlsMinVersionFromEnv()
	if err != nil {
		return Config{}, err
	}
	ciphers, err := tlsCipherSuitesFromEnv()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddr:              stringFromEnv(EnvHTTPAddr, DefaultHTTPAddr),
		HTTPSAddr:             stringFromEnv(EnvHTTPSAddr, DefaultHTTPSAddr),
		TLSCertFile:           stringFromEnv(EnvTLSCertFile, DefaultTLSCertFile),
		TLSKeyFile:            stringFromEnv(EnvTLSKeyFile, DefaultTLSKeyFile),
		TLSCAFile:             stringFromEnv(EnvTLSCAFile, DefaultTLSCAFile),
		TLSCADir:              stringFromEnv(EnvTLSCADir, DefaultTLSCADir),
		TLSCABundleFile:       stringFromEnv(EnvTLSCABundleFile, DefaultTLSCABundle),
		TLSClientCAFile:       strings.TrimSpace(os.Getenv(EnvTLSClientCAFile)),
		TLSDNSNames:           splitCSV(stringFromEnv(EnvTLSDNSNames, DefaultDNSNames)),
		DataDir:               stringFromEnv(EnvDataDir, DefaultDataDir),
		MaxBlobBytes:          maxBytes,
		ResultsLog:            stringFromEnv(EnvResultsLog, DefaultResultsLog),
		WriteToken:            strings.TrimSpace(os.Getenv(EnvWriteToken)),
		ServeCA:               serveCA,
		TLSMinVersion:         minVersion,
		TLSCipherSuites:       ciphers,
		DisableSessionTickets: disableTickets,
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

func boolFromEnv(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", key)
	}
}

func tlsMinVersionFromEnv() (uint16, error) {
	raw := strings.TrimSpace(os.Getenv(EnvTLSMinVersion))
	if raw == "" {
		return tls.VersionTLS12, nil
	}
	switch strings.ToLower(strings.ReplaceAll(raw, " ", "")) {
	case "1.2", "tls1.2", "tlsv1.2":
		return tls.VersionTLS12, nil
	case "1.3", "tls1.3", "tlsv1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("%s must be 1.2 or 1.3", EnvTLSMinVersion)
	}
}

func tlsCipherSuitesFromEnv() ([]uint16, error) {
	raw := strings.TrimSpace(os.Getenv(EnvTLSCipherSuites))
	if raw == "" {
		return nil, nil
	}
	names := splitCSV(raw)
	ids := make([]uint16, 0, len(names))
	known := cipherSuiteByName()
	for _, name := range names {
		id, ok := known[name]
		if !ok {
			return nil, fmt.Errorf("%s: unknown cipher suite %q (use Go IANA names such as TLS_AES_128_GCM_SHA256)", EnvTLSCipherSuites, name)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func cipherSuiteByName() map[string]uint16 {
	suites := tls.CipherSuites()
	insecure := tls.InsecureCipherSuites()
	out := make(map[string]uint16, len(suites)+len(insecure))
	for _, suite := range suites {
		out[suite.Name] = suite.ID
	}
	for _, suite := range insecure {
		out[suite.Name] = suite.ID
	}
	return out
}

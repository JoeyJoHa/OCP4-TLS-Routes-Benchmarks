package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
)

const (
	pemCertType = "CERTIFICATE"
	pemKeyType  = "EC PRIVATE KEY"
	filePerm    = 0o600
	certPerm    = 0o644
	dirPerm     = 0o750
)

// Material holds the TLS files the server presents and the CA clients can trust.
type Material struct {
	Certificate tls.Certificate
	CAPEM       []byte
	CertPEM     []byte
	Generated   bool
}

// LoadOrGenerate uses existing cert/key files, or writes a lab CA plus server cert.
func LoadOrGenerate(cfg config.Config) (Material, error) {
	certExists := fileExists(cfg.TLSCertFile)
	keyExists := fileExists(cfg.TLSKeyFile)
	if certExists && keyExists {
		return loadProvided(cfg)
	}
	if certExists != keyExists {
		return Material{}, fmt.Errorf("both %s and %s must exist, or neither", cfg.TLSCertFile, cfg.TLSKeyFile)
	}
	return generateAndWrite(cfg)
}

func loadProvided(cfg config.Config) (Material, error) {
	pair, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return Material{}, fmt.Errorf("load server certificate: %w", err)
	}
	attachLeaf(&pair)
	certPEM, err := os.ReadFile(cfg.TLSCertFile)
	if err != nil {
		return Material{}, err
	}
	caPEM, err := readOptional(cfg.TLSCAFile)
	if err != nil {
		return Material{}, err
	}
	if len(caPEM) == 0 {
		caPEM = certPEM
	}
	return Material{
		Certificate: pair,
		CAPEM:       caPEM,
		CertPEM:     certPEM,
		Generated:   false,
	}, nil
}

func generateAndWrite(cfg config.Config) (Material, error) {
	now := time.Now()
	notAfter := now.Add(time.Duration(config.CertValidityDays) * 24 * time.Hour)

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Material{}, fmt.Errorf("generate CA key: %w", err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Material{}, fmt.Errorf("generate server key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          serialNumber(),
		Subject:               pkix.Name{Organization: []string{"OCP4 TLS Bench"}, CommonName: "tlsbench-internal-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return Material{}, fmt.Errorf("create CA certificate: %w", err)
	}

	dnsNames, ipAddrs := splitNames(cfg.TLSDNSNames)
	serverTemplate := &x509.Certificate{
		SerialNumber:          serialNumber(),
		Subject:               pkix.Name{Organization: []string{"OCP4 TLS Bench"}, CommonName: "tlsbench"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return Material{}, fmt.Errorf("create server certificate: %w", err)
	}

	caPEM := pem.EncodeToMemory(&pem.Block{Type: pemCertType, Bytes: caDER})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: pemCertType, Bytes: serverDER})
	keyPEM, err := encodeKey(serverKey)
	if err != nil {
		return Material{}, err
	}
	caKeyPEM, err := encodeKey(caKey)
	if err != nil {
		return Material{}, err
	}

	if err := writeFile(cfg.TLSCAFile, caPEM, certPerm); err != nil {
		return Material{}, err
	}
	if err := writeFile(caKeyPath(cfg.TLSCAFile), caKeyPEM, filePerm); err != nil {
		return Material{}, err
	}
	if err := writeFile(cfg.TLSCertFile, certPEM, certPerm); err != nil {
		return Material{}, err
	}
	if err := writeFile(cfg.TLSKeyFile, keyPEM, filePerm); err != nil {
		return Material{}, err
	}

	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return Material{}, err
	}
	attachLeaf(&pair)
	return Material{
		Certificate: pair,
		CAPEM:       caPEM,
		CertPEM:     certPEM,
		Generated:   true,
	}, nil
}

func attachLeaf(cert *tls.Certificate) {
	if cert == nil || len(cert.Certificate) == 0 || cert.Leaf != nil {
		return
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return
	}
	cert.Leaf = leaf
}

func encodeKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: pemKeyType, Bytes: der}), nil
}

func splitNames(names []string) ([]string, []net.IP) {
	dns := make([]string, 0, len(names)+2)
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	seenDNS := map[string]struct{}{}
	seenIP := map[string]struct{}{"127.0.0.1": {}, "::1": {}}
	for _, name := range names {
		if ip := net.ParseIP(name); ip != nil {
			key := ip.String()
			if _, exists := seenIP[key]; exists {
				continue
			}
			seenIP[key] = struct{}{}
			ips = append(ips, ip)
			continue
		}
		if _, exists := seenDNS[name]; exists {
			continue
		}
		seenDNS[name] = struct{}{}
		dns = append(dns, name)
	}
	if host, err := os.Hostname(); err == nil && host != "" {
		if _, exists := seenDNS[host]; !exists {
			dns = append(dns, host)
		}
	}
	if len(dns) == 0 {
		dns = []string{"localhost"}
	}
	return dns, ips
}

func serialNumber() *big.Int {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return n
}

func writeFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func readOptional(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if os.IsNotExist(err) {
		return nil, nil
	}
	return nil, err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func caKeyPath(caFile string) string {
	dir := filepath.Dir(caFile)
	base := filepath.Base(caFile)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	if name == "" {
		name = "ca"
	}
	return filepath.Join(dir, name+".key")
}

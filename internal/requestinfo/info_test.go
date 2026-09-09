package requestinfo

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFromRequestHTTPHasNoTLS(t *testing.T) {
	req := httptest.NewRequest("GET", "http://tlsbench.apps.example.com/api/info", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "192.0.2.10, 10.0.0.1")
	req.RemoteAddr = "10.0.0.1:443"
	info := FromRequest(req, "tlsbench-pod", nil)
	if info.TLS {
		t.Fatal("edge HTTP must not set tls")
	}
	if info.ClientAddr != "192.0.2.10" {
		t.Fatalf("client=%s", info.ClientAddr)
	}
	if info.XForwardedProto != "https" {
		t.Fatalf("proto=%s", info.XForwardedProto)
	}
	if info.CertKeyAlgorithm != "" {
		t.Fatalf("HTTP without a server cert must not report a key: %+v", info)
	}
}

func TestFromRequestTLS(t *testing.T) {
	req := httptest.NewRequest("GET", "https://tlsbench.svc/api/info", nil)
	req.RemoteAddr = "10.128.0.12:45678"
	req.TLS = &tls.ConnectionState{
		Version:            tls.VersionTLS13,
		CipherSuite:        tls.TLS_AES_128_GCM_SHA256,
		ServerName:         "tlsbench.svc",
		NegotiatedProtocol: "http/1.1",
	}
	info := FromRequest(req, "tlsbench-pod", nil)
	if !info.TLS || info.TLSVersion != "TLS 1.3" {
		t.Fatalf("tls info: %+v", info)
	}
	if info.ClientAddr != "10.128.0.12" {
		t.Fatalf("client=%s", info.ClientAddr)
	}
	if info.SNI != "tlsbench.svc" {
		t.Fatalf("sni=%s", info.SNI)
	}
	if info.Cipher != "TLS_AES_128_GCM_SHA256" {
		t.Fatalf("cipher=%s", info.Cipher)
	}
}

func TestFromRequestECDSACertKey(t *testing.T) {
	cert := mustSelfSigned(t, ecdsaP256Key(t))
	req := httptest.NewRequest("GET", "https://tlsbench.svc/api/info", nil)
	req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, CipherSuite: tls.TLS_AES_128_GCM_SHA256}
	info := FromRequest(req, "tlsbench-pod", cert)
	if info.CertKeyAlgorithm != "ECDSA" {
		t.Fatalf("algorithm=%s", info.CertKeyAlgorithm)
	}
	if info.CertKeySize != 256 {
		t.Fatalf("size=%d", info.CertKeySize)
	}
	if info.CertKeyCurve != "P-256" {
		t.Fatalf("curve=%s", info.CertKeyCurve)
	}
}

func TestFromRequestRSACertKey(t *testing.T) {
	cert := mustSelfSigned(t, rsa2048Key(t))
	req := httptest.NewRequest("GET", "https://tlsbench.svc/api/info", nil)
	req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, CipherSuite: tls.TLS_AES_128_GCM_SHA256}
	info := FromRequest(req, "tlsbench-pod", cert)
	if info.CertKeyAlgorithm != "RSA" {
		t.Fatalf("algorithm=%s", info.CertKeyAlgorithm)
	}
	if info.CertKeySize != 2048 {
		t.Fatalf("size=%d", info.CertKeySize)
	}
	if info.CertKeyCurve != "" {
		t.Fatalf("curve=%s", info.CertKeyCurve)
	}
}

func ecdsaP256Key(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func rsa2048Key(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustSelfSigned(t *testing.T, key any) *tls.Certificate {
	t.Helper()
	var pub any
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		pub = &k.PublicKey
	case *rsa.PrivateKey:
		pub = &k.PublicKey
	default:
		t.Fatalf("unsupported key %T", key)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "tlsbench"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"tlsbench.svc"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

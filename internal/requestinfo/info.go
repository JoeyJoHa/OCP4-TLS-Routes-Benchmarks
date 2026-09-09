package requestinfo

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"strings"
	"time"
)

// Info describes how this request reached the pod (TLS vs edge HTTP).
type Info struct {
	TLS              bool      `json:"tls"`
	TLSVersion       string    `json:"tls_version,omitempty"`
	Cipher           string    `json:"cipher,omitempty"`
	SNI              string    `json:"sni,omitempty"`
	ALPN             string    `json:"alpn,omitempty"`
	Host             string    `json:"host"`
	Method           string    `json:"method"`
	Path             string    `json:"path"`
	RemoteAddr       string    `json:"remote_addr"`
	ClientAddr       string    `json:"client_addr"`
	XForwardedFor    string    `json:"x_forwarded_for,omitempty"`
	XForwardedProto  string    `json:"x_forwarded_proto,omitempty"`
	XForwardedHost   string    `json:"x_forwarded_host,omitempty"`
	ServerName       string    `json:"server_cert_cn,omitempty"`
	CertSANs         []string  `json:"server_cert_sans,omitempty"`
	CertNotAfter     time.Time `json:"server_cert_not_after,omitempty"`
	CertKeyAlgorithm string    `json:"server_cert_key_algorithm,omitempty"`
	CertKeySize      int       `json:"server_cert_key_size,omitempty"`
	CertKeyCurve     string    `json:"server_cert_curve,omitempty"`
	Hostname         string    `json:"hostname,omitempty"`
}

func FromRequest(r *http.Request, hostname string, serverTLS *tls.Certificate) Info {
	info := Info{
		Host:            r.Host,
		Method:          r.Method,
		Path:            r.URL.Path,
		RemoteAddr:      r.RemoteAddr,
		XForwardedFor:   r.Header.Get("X-Forwarded-For"),
		XForwardedProto: r.Header.Get("X-Forwarded-Proto"),
		XForwardedHost:  r.Header.Get("X-Forwarded-Host"),
		Hostname:        hostname,
		ClientAddr:      clientAddr(r),
	}
	if r.TLS != nil {
		info.TLS = true
		info.TLSVersion = tlsVersion(r.TLS.Version)
		info.Cipher = tls.CipherSuiteName(r.TLS.CipherSuite)
		info.SNI = r.TLS.ServerName
		info.ALPN = r.TLS.NegotiatedProtocol
	}
	fillCertFields(&info, serverTLS)
	return info
}

func fillCertFields(info *Info, cert *tls.Certificate) {
	leaf := leafFrom(cert)
	if leaf == nil {
		return
	}
	info.ServerName = leaf.Subject.CommonName
	info.CertSANs = append([]string{}, leaf.DNSNames...)
	info.CertNotAfter = leaf.NotAfter.UTC()
	info.CertKeyAlgorithm, info.CertKeySize, info.CertKeyCurve = publicKeyMeta(leaf)
}

func publicKeyMeta(leaf *x509.Certificate) (algorithm string, bits int, curve string) {
	switch pub := leaf.PublicKey.(type) {
	case *rsa.PublicKey:
		return "RSA", pub.N.BitLen(), ""
	case *ecdsa.PublicKey:
		if pub.Curve != nil && pub.Curve.Params() != nil {
			return "ECDSA", pub.Curve.Params().BitSize, pub.Curve.Params().Name
		}
		return "ECDSA", 0, ""
	case ed25519.PublicKey:
		return "Ed25519", len(pub) * 8, ""
	default:
		if leaf.PublicKeyAlgorithm == x509.UnknownPublicKeyAlgorithm {
			return "", 0, ""
		}
		return leaf.PublicKeyAlgorithm.String(), 0, ""
	}
}

func leafFrom(cert *tls.Certificate) *x509.Certificate {
	if cert == nil || len(cert.Certificate) == 0 {
		return nil
	}
	if cert.Leaf != nil {
		return cert.Leaf
	}
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil
	}
	return parsed
}

func clientAddr(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func tlsVersion(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	default:
		return ""
	}
}

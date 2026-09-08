package requestinfo

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
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
}

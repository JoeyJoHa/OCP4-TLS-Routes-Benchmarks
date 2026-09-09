package timing

import "testing"

func TestPhasesFromCurlHTTPS(t *testing.T) {
	phases := PhasesFromCurl(CurlTimes{
		Namelookup:    0.001,
		Connect:       0.004,
		Appconnect:    0.054,
		Pretransfer:   0.055,
		Starttransfer: 0.060,
		Redirect:      0,
		Total:         0.160,
	})
	assertMs(t, "dns", phases.DNSMs, 1)
	assertMs(t, "tcp", phases.TCPConnectMs, 3)
	assertMs(t, "tls", phases.TLSHandshakeMs, 50)
	assertMs(t, "ttfb", phases.TTFBMs, 5)
	assertMs(t, "xfer", phases.TransferMs, 100)
	assertMs(t, "total", phases.ClientTotalMs, 160)
}

func TestPhasesFromCurlHTTPHasNoHandshake(t *testing.T) {
	phases := PhasesFromCurl(CurlTimes{
		Namelookup:    0.002,
		Connect:       0.006,
		Appconnect:    0,
		Pretransfer:   0.007,
		Starttransfer: 0.010,
		Total:         0.040,
	})
	if phases.TLSHandshakeMs != 0 {
		t.Fatalf("tls=%v", phases.TLSHandshakeMs)
	}
	assertMs(t, "tcp", phases.TCPConnectMs, 4)
	assertMs(t, "ttfb", phases.TTFBMs, 3)
	assertMs(t, "xfer", phases.TransferMs, 30)
}

func assertMs(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got != want {
		t.Fatalf("%s=%v want %v", name, got, want)
	}
}

package timing

import "testing"

func TestPhasesFromCurl(t *testing.T) {
	tests := []struct {
		name string
		in   CurlTimes
		want Phases
	}{
		{
			name: "https handshake and body",
			in: CurlTimes{
				Namelookup:    0.001,
				Connect:       0.004,
				Appconnect:    0.054,
				Pretransfer:   0.055,
				Starttransfer: 0.060,
				Redirect:      0,
				Total:         0.160,
			},
			want: Phases{
				DNSMs:          1,
				TCPConnectMs:   3,
				TLSHandshakeMs: 50,
				TTFBMs:         5,
				TransferMs:     100,
				ClientTotalMs:  160,
			},
		},
		{
			name: "http has no handshake",
			in: CurlTimes{
				Namelookup:    0.002,
				Connect:       0.006,
				Appconnect:    0,
				Pretransfer:   0.007,
				Starttransfer: 0.010,
				Total:         0.040,
			},
			want: Phases{
				DNSMs:         2,
				TCPConnectMs:  4,
				TTFBMs:        3,
				TransferMs:    30,
				ClientTotalMs: 40,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PhasesFromCurl(tt.in)
			assertMs(t, "dns", got.DNSMs, tt.want.DNSMs)
			assertMs(t, "tcp", got.TCPConnectMs, tt.want.TCPConnectMs)
			assertMs(t, "tls", got.TLSHandshakeMs, tt.want.TLSHandshakeMs)
			assertMs(t, "ttfb", got.TTFBMs, tt.want.TTFBMs)
			assertMs(t, "xfer", got.TransferMs, tt.want.TransferMs)
			assertMs(t, "total", got.ClientTotalMs, tt.want.ClientTotalMs)
		})
	}
}

func assertMs(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got != want {
		t.Fatalf("%s=%v want %v", name, got, want)
	}
}

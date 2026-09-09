package timing

import "math"

const millisecondsPerSecond = 1000.0

// CurlTimes is curl --write-out timing fields, in seconds.
type CurlTimes struct {
	Namelookup    float64 `json:"time_namelookup"`
	Connect       float64 `json:"time_connect"`
	Appconnect    float64 `json:"time_appconnect"`
	Pretransfer   float64 `json:"time_pretransfer"`
	Starttransfer float64 `json:"time_starttransfer"`
	Redirect      float64 `json:"time_redirect"`
	Total         float64 `json:"time_total"`
}

// Phases is the non-overlapping breakdown used in the dashboard.
type Phases struct {
	DNSMs          float64
	TCPConnectMs   float64
	TLSHandshakeMs float64
	RedirectMs     float64
	TTFBMs         float64
	TransferMs     float64
	ClientTotalMs  float64
}

// PhasesFromCurl converts curl's cumulative seconds into phase milliseconds.
func PhasesFromCurl(times CurlTimes) Phases {
	namelookup := secondsToMs(times.Namelookup)
	connect := secondsToMs(times.Connect)
	appconnect := secondsToMs(times.Appconnect)
	pretransfer := secondsToMs(times.Pretransfer)
	starttransfer := secondsToMs(times.Starttransfer)
	total := secondsToMs(times.Total)
	return Phases{
		DNSMs:          namelookup,
		TCPConnectMs:   delta(connect, namelookup),
		TLSHandshakeMs: tlsDelta(appconnect, connect),
		RedirectMs:     secondsToMs(times.Redirect),
		TTFBMs:         delta(starttransfer, pretransfer),
		TransferMs:     delta(total, starttransfer),
		ClientTotalMs:  total,
	}
}

func tlsDelta(appconnect, connect float64) float64 {
	if appconnect <= 0 {
		return 0
	}
	return delta(appconnect, connect)
}

func delta(end, start float64) float64 {
	diff := end - start
	if diff < 0 {
		return 0
	}
	return math.Round(diff*1000) / 1000
}

func secondsToMs(seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return math.Round(seconds*millisecondsPerSecond*1000) / 1000
}

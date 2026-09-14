package timing

import "sort"

// Summary holds percentile statistics for one metric (milliseconds).
type Summary struct {
	Count int     `json:"count"`
	Min   float64 `json:"min_ms"`
	Max   float64 `json:"max_ms"`
	Mean  float64 `json:"mean_ms"`
	P50   float64 `json:"p50_ms"`
	P90   float64 `json:"p90_ms"`
	P99   float64 `json:"p99_ms"`
}

// Summarize returns percentile stats for non-zero samples.
func Summarize(values []float64) Summary {
	filtered := make([]float64, 0, len(values))
	var sum float64
	for _, v := range values {
		if v <= 0 {
			continue
		}
		filtered = append(filtered, v)
		sum += v
	}
	if len(filtered) == 0 {
		return Summary{}
	}
	sort.Float64s(filtered)
	return Summary{
		Count: len(filtered),
		Min:   filtered[0],
		Max:   filtered[len(filtered)-1],
		Mean:  sum / float64(len(filtered)),
		P50:   percentile(filtered, 50),
		P90:   percentile(filtered, 90),
		P99:   percentile(filtered, 99),
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := (p / 100) * float64(len(sorted)-1)
	lower := int(rank)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	weight := rank - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

package timing

import "testing"

func TestSummarizePercentiles(t *testing.T) {
	summary := Summarize([]float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100})
	if summary.Count != 10 {
		t.Fatalf("count=%d", summary.Count)
	}
	if summary.Min != 10 || summary.Max != 100 {
		t.Fatalf("min/max=%v/%v", summary.Min, summary.Max)
	}
	if summary.P50 != 55 {
		t.Fatalf("p50=%v", summary.P50)
	}
	if summary.P90 != 91 {
		t.Fatalf("p90=%v", summary.P90)
	}
}

func TestSummarizeSkipsZeros(t *testing.T) {
	summary := Summarize([]float64{0, 5, 0, 15})
	if summary.Count != 2 || summary.Mean != 10 {
		t.Fatalf("summary=%+v", summary)
	}
}

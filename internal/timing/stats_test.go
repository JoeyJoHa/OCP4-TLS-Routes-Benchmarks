package timing

import "testing"

func TestSummarize(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want Summary
	}{
		{
			name: "percentiles",
			in:   []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
			want: Summary{Count: 10, Min: 10, Max: 100, P50: 55, P90: 91},
		},
		{
			name: "skips zeros",
			in:   []float64{0, 5, 0, 15},
			want: Summary{Count: 2, Mean: 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize(tt.in)
			if got.Count != tt.want.Count {
				t.Fatalf("count=%d want %d", got.Count, tt.want.Count)
			}
			if tt.want.Min != 0 && (got.Min != tt.want.Min || got.Max != tt.want.Max) {
				t.Fatalf("min/max=%v/%v want %v/%v", got.Min, got.Max, tt.want.Min, tt.want.Max)
			}
			if tt.want.P50 != 0 && got.P50 != tt.want.P50 {
				t.Fatalf("p50=%v want %v", got.P50, tt.want.P50)
			}
			if tt.want.P90 != 0 && got.P90 != tt.want.P90 {
				t.Fatalf("p90=%v want %v", got.P90, tt.want.P90)
			}
			if tt.want.Mean != 0 && got.Mean != tt.want.Mean {
				t.Fatalf("mean=%v want %v", got.Mean, tt.want.Mean)
			}
		})
	}
}

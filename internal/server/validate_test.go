package server

import (
	"strings"
	"testing"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
)

func TestValidExperimentID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "simple", id: "exp1", want: true},
		{name: "colon and dash", id: "vm-1710000000:1", want: true},
		{name: "empty", id: "", want: false},
		{name: "html", id: "<script>", want: false},
		{name: "space", id: "exp 1", want: false},
		{name: "too long", id: strings.Repeat("a", config.MaxExperimentIDLength+1), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validExperimentID(tt.id)
			if got != tt.want {
				t.Fatalf("validExperimentID(%q)=%v want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestParseRouteMode(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "empty", raw: "", want: "", ok: true},
		{name: "trimmed passthrough", raw: "  passthrough ", want: "passthrough", ok: true},
		{name: "edge", raw: "edge", want: "edge", ok: true},
		{name: "reencrypt", raw: "reencrypt", want: "reencrypt", ok: true},
		{name: "service-http", raw: "service-http", want: "service-http", ok: true},
		{name: "service-https", raw: "service-https", want: "service-https", ok: true},
		{name: "unknown", raw: "http", want: "", ok: false},
		{name: "html", raw: "<b>edge</b>", want: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRouteMode(tt.raw)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("parseRouteMode(%q)=%q %v want %q %v", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

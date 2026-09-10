package server

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
)

var experimentIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

var allowedRouteModes = map[string]struct{}{
	"edge":          {},
	"passthrough":   {},
	"reencrypt":     {},
	"service-http":  {},
	"service-https": {},
}

func validExperimentID(id string) bool {
	if id == "" || utf8.RuneCountInString(id) > config.MaxExperimentIDLength {
		return false
	}
	return experimentIDPattern.MatchString(id)
}

func parseRouteMode(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	if _, ok := allowedRouteModes[raw]; ok {
		return raw, true
	}
	return "", false
}

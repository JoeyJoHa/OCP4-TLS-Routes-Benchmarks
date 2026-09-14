package server

import (
	"net/http"
	"strconv"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/results"
)

func (a *App) benchProbe(w http.ResponseWriter, r *http.Request) {
	experimentID := r.URL.Query().Get("experiment_id")
	if !validExperimentID(experimentID) {
		writeError(w, http.StatusBadRequest, "invalid experiment_id")
		return
	}
	sampleIndex, err := strconv.Atoi(r.URL.Query().Get("sample_index"))
	if err != nil || sampleIndex <= 0 {
		writeError(w, http.StatusBadRequest, "sample_index must be a positive integer")
		return
	}
	routeMode, ok := parseRouteMode(r.URL.Query().Get("route_mode"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid route_mode")
		return
	}
	run := results.Run{
		Operation:    "handshake",
		Name:         probeName(experimentID, sampleIndex),
		ExperimentID: experimentID,
		SampleIndex:  sampleIndex,
		RouteMode:    routeMode,
	}
	a.record(r, run)
	a.apiInfo(w, r)
}

func probeName(experimentID string, sampleIndex int) string {
	return experimentID + "-" + strconv.Itoa(sampleIndex)
}

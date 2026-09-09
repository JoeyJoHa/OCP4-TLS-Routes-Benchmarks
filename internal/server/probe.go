package server

import (
	"net/http"
	"strconv"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/results"
)

func (a *App) benchProbe(w http.ResponseWriter, r *http.Request) {
	experimentID := r.URL.Query().Get("experiment_id")
	if experimentID == "" {
		writeError(w, http.StatusBadRequest, "experiment_id is required")
		return
	}
	sampleIndex, err := strconv.Atoi(r.URL.Query().Get("sample_index"))
	if err != nil || sampleIndex <= 0 {
		writeError(w, http.StatusBadRequest, "sample_index must be a positive integer")
		return
	}
	run := results.Run{
		Operation:    "handshake",
		Name:         probeName(experimentID, sampleIndex),
		ExperimentID: experimentID,
		SampleIndex:  sampleIndex,
		RouteMode:    r.URL.Query().Get("route_mode"),
	}
	a.record(r, run)
	a.apiInfo(w, r)
}

func probeName(experimentID string, sampleIndex int) string {
	return experimentID + "-" + strconv.Itoa(sampleIndex)
}

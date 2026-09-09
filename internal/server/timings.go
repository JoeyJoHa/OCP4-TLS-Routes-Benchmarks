package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/results"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/timing"
)

type clientTimingsRequest struct {
	Operation    string `json:"operation"`
	Name         string `json:"name"`
	RouteMode    string `json:"route_mode,omitempty"`
	ExperimentID string `json:"experiment_id,omitempty"`
	SampleIndex  int    `json:"sample_index,omitempty"`
	timing.CurlTimes
}

func (a *App) attachClientTimings(w http.ResponseWriter, r *http.Request) {
	var req clientTimingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !validTimingOperation(req.Operation) {
		writeError(w, http.StatusBadRequest, "invalid operation")
		return
	}
	meta := results.ClientTimingMeta{
		RouteMode:    req.RouteMode,
		ExperimentID: req.ExperimentID,
		SampleIndex:  req.SampleIndex,
	}
	name := req.Name
	if meta.ExperimentID != "" && meta.SampleIndex > 0 {
		name = probeName(meta.ExperimentID, meta.SampleIndex)
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "name or experiment_id with sample_index is required")
		return
	}
	run, err := a.log.MergeClientTimings(name, req.Operation, timing.PhasesFromCurl(req.CurlTimes), meta)
	if err != nil {
		if errors.Is(err, results.ErrNoMatchingRun) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func validTimingOperation(operation string) bool {
	switch operation {
	case "generate", "upload", "download", "handshake":
		return true
	default:
		return false
	}
}

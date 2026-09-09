package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/results"
	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/timing"
)

type clientTimingsRequest struct {
	Operation string `json:"operation"`
	Name      string `json:"name"`
	timing.CurlTimes
}

func (a *App) attachClientTimings(w http.ResponseWriter, r *http.Request) {
	var req clientTimingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !validTimingOperation(req.Operation) || req.Name == "" {
		writeError(w, http.StatusBadRequest, "operation and name are required")
		return
	}
	run, err := a.log.MergeClientTimings(req.Name, req.Operation, timing.PhasesFromCurl(req.CurlTimes))
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
	case "generate", "upload", "download":
		return true
	default:
		return false
	}
}

package server

import (
	"encoding/json"
	"log"
	"net/http"
)

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := encodeJSON(w, payload); err != nil {
		log.Printf("write json: %v", err)
	}
}

func encodeJSON(w http.ResponseWriter, payload any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	return enc.Encode(payload)
}

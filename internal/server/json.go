package server

import (
	"encoding/json"
	"net/http"
)

func encodeJSON(w http.ResponseWriter, payload any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	return enc.Encode(payload)
}

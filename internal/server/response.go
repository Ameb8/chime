package server

import (
	"encoding/json"
	"net/http"
)

type responseEnvelope struct {
	OK    bool   `json:"ok"`
	Event string `json:"event,omitempty"`
	Error string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, responseEnvelope{
		OK:    false,
		Error: message,
	})
}

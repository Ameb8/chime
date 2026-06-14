package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Ameb8/chime/internal/notify"
)

const (
	maxRequestBodyBytes = 1 << 20
	maxAgentRunes       = 64
	maxMessageRunes     = 512
)

type notifyRequest struct {
	Event   string `json:"event"`
	Agent   string `json:"agent"`
	Message string `json:"message"`
}

type healthResponse struct {
	OK            bool   `json:"ok"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

func (s *Server) notifyHandler(w http.ResponseWriter, r *http.Request) {
	var req notifyRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	if err := decoder.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusBadRequest, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "malformed json")
		return
	}

	event, err := parseEvent(req.Event)
	if err != nil {
		if errors.Is(err, errUnknownEvent) {
			writeError(w, http.StatusUnprocessableEntity, "unknown event type")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	notification := notify.Notification{
		Event:   event,
		Agent:   truncateRunes(strings.TrimSpace(req.Agent), maxAgentRunes),
		Message: truncateRunes(strings.TrimSpace(req.Message), maxMessageRunes),
	}

	if err := s.dispatcher.Dispatch(r.Context(), notification); err != nil {
		writeError(w, http.StatusInternalServerError, "dispatch failed")
		return
	}

	writeJSON(w, http.StatusOK, responseEnvelope{
		OK:    true,
		Event: string(event),
	})
}

func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	uptime := int64(time.Since(s.startedAt).Seconds())
	if uptime < 0 {
		uptime = 0
	}

	writeJSON(w, http.StatusOK, healthResponse{
		OK:            true,
		Version:       s.opts.Version,
		UptimeSeconds: uptime,
	})
}

var errUnknownEvent = errors.New("unknown event type")

func parseEvent(raw string) (notify.Event, error) {
	event := strings.TrimSpace(raw)
	if event == "" {
		return "", errors.New("missing event field")
	}

	switch notify.Event(event) {
	case notify.EventComplete:
		return notify.EventComplete, nil
	case notify.EventWaiting:
		return notify.EventWaiting, nil
	default:
		return "", errUnknownEvent
	}
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

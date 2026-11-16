package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"example.com/specflow/backend/internal/orchestrator"
)

const (
	defaultSSEHeartbeat    = 15 * time.Second
	defaultSSEWriteTimeout = 30 * time.Second
)

// These are variables so tests can shorten the heartbeat without waiting for
// the production interval.
var (
	sseHeartbeatInterval = defaultSSEHeartbeat
	sseWriteTimeout      = defaultSSEWriteTimeout
)

// events streams a run's events. A sealed run replays and closes; a live run
// replays what the client missed and then follows it with a heartbeat.
func (h *runHandlers) events(w http.ResponseWriter, r *http.Request) {
	project, runID, ok := runTarget(w, r)
	if !ok {
		return
	}

	after := lastEventID(r)
	stream, err := h.engine.Subscribe(r.Context(), project, runID, after)
	if err != nil {
		if errors.Is(err, orchestrator.ErrRunNotFound) {
			writeRunError(w, err)
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	defer stream.Cancel()

	// A terminal run whose client already has the tail has nothing to send.
	// 204 is the EventSource stop signal, so the client does not reconnect.
	if stream.Sealed && len(stream.Replay) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	controller := http.NewResponseController(w)
	writeEvent := func(event orchestrator.Event) error {
		_ = controller.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Seq, event.Type, payload); err != nil {
			return err
		}
		return controller.Flush()
	}
	writeHeartbeat := func() error {
		_ = controller.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
		if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
			return err
		}
		return controller.Flush()
	}

	for _, event := range stream.Replay {
		if err := writeEvent(event); err != nil {
			return
		}
	}
	if stream.Sealed {
		return
	}

	ticker := time.NewTicker(sseHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case event, open := <-stream.Events:
			if !open {
				return
			}
			if err := writeEvent(event); err != nil {
				return
			}
		case <-ticker.C:
			if err := writeHeartbeat(); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func lastEventID(r *http.Request) uint64 {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("after")
	}
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

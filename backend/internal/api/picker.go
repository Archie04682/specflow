package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"example.com/specflow/backend/internal/picker"
)

// pickerDeadline bounds a request while the operator is looking at the dialog.
// A dismissed or hung dialog must not hold a connection forever.
const pickerDeadline = 2 * time.Minute

// pickFolder is a seam: tests replace it so no dialog opens during `make test`.
var pickFolder = picker.Pick

// projectPicker opens the operating system's folder dialog on the local
// machine. It is deliberately not a directory-listing endpoint: one path out,
// nothing enumerated, and nothing in the project changed.
func projectPicker(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackAddress(r.RemoteAddr) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "the folder dialog is only available to loopback clients"})
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && !isLoopbackOrigin(origin) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "the folder dialog is only available to loopback origins"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), pickerDeadline)
	defer cancel()

	path, err := pickFolder(ctx)
	switch {
	case errors.Is(err, picker.ErrCancelled):
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
	case errors.Is(err, picker.ErrUnsupported):
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": err.Error()})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"path": path})
	}
}

func isLoopbackAddress(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isLoopbackOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"example.com/specflow/backend/internal/buildinfo"
)

func requestServer(t *testing.T, router http.Handler) (int, map[string]json.RawMessage, http.Header) {
	t.Helper()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/server", nil))

	var body map[string]json.RawMessage
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v\nbody: %s", err, rec.Body.String())
		}
	}
	return rec.Code, body, rec.Header()
}

func rawString(t *testing.T, body map[string]json.RawMessage, key string) string {
	t.Helper()
	value, ok := body[key]
	if !ok {
		t.Fatalf("body has no %q field: %v", key, keys(body))
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		t.Fatalf("field %q is not a string: %v", key, err)
	}
	return text
}

func keys(body map[string]json.RawMessage) []string {
	out := make([]string, 0, len(body))
	for key := range body {
		out = append(out, key)
	}
	return out
}

func TestServerIdentityNamesTheRunningBuild(t *testing.T) {
	modified := true
	timestamp := time.Date(2026, 11, 15, 10, 7, 33, 0, time.UTC)
	identity := ServerIdentity{
		Info: buildinfo.Info{
			Version:      "0.2.0",
			Revision:     "abc123",
			RevisionTime: timestamp,
			Modified:     &modified,
			BuiltAt:      timestamp.Add(time.Hour),
		},
		StartedAt:    timestamp.Add(2 * time.Hour),
		CheckoutRoot: "/checkout",
		Probe: func(string, buildinfo.Info, time.Time) (*buildinfo.Checkout, error) {
			return &buildinfo.Checkout{
				Revision:     "def456",
				StaleReason:  buildinfo.ReasonCheckoutAhead,
				NewerSources: 2,
			}, nil
		},
	}

	status, body, header := requestServer(t, NewRouterWithServer(fstest.MapFS{}, nil, nil, identity))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if got := header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := rawString(t, body, "service"); got != "specflow" {
		t.Fatalf("service = %q, want specflow", got)
	}
	if got := rawString(t, body, "version"); got != "0.2.0" {
		t.Fatalf("version = %q, want 0.2.0", got)
	}
	if got := rawString(t, body, "revision"); got != "abc123" {
		t.Fatalf("revision = %q, want abc123", got)
	}
	if got := rawString(t, body, "revisionTime"); got != "2026-11-15T10:07:33Z" {
		t.Fatalf("revisionTime = %q, want the UTC stamp", got)
	}
	if got := rawString(t, body, "builtAt"); got != "2026-11-15T11:07:33Z" {
		t.Fatalf("builtAt = %q, want the UTC build time", got)
	}
	if got := rawString(t, body, "startedAt"); got != "2026-11-15T12:07:33Z" {
		t.Fatalf("startedAt = %q, want the UTC start time", got)
	}

	var modifiedValue bool
	if err := json.Unmarshal(body["modified"], &modifiedValue); err != nil || !modifiedValue {
		t.Fatalf("modified = %s, want true", body["modified"])
	}

	var checkout struct {
		Revision     string `json:"revision"`
		StaleReason  string `json:"staleReason"`
		NewerSources int    `json:"newerSources"`
	}
	if err := json.Unmarshal(body["checkout"], &checkout); err != nil {
		t.Fatalf("decode checkout: %v", err)
	}
	if checkout.Revision != "def456" || checkout.StaleReason != buildinfo.ReasonCheckoutAhead {
		t.Fatalf("checkout = %+v, want the probed revision and reason", checkout)
	}
	if checkout.NewerSources != 2 {
		t.Fatalf("newerSources = %d, want the probed count", checkout.NewerSources)
	}
}

func TestServerIdentityOmitsUnknownStamps(t *testing.T) {
	status, body, _ := requestServer(t, NewRouterWithServer(fstest.MapFS{}, nil, nil, ServerIdentity{}))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if len(body) != 1 {
		t.Fatalf("body = %v, want only the service name", keys(body))
	}
	if got := rawString(t, body, "service"); got != "specflow" {
		t.Fatalf("service = %q, want specflow", got)
	}
	for _, omitted := range []string{"version", "revision", "revisionTime", "modified", "builtAt", "startedAt", "checkout"} {
		if _, ok := body[omitted]; ok {
			t.Fatalf("unknown field %q was published", omitted)
		}
	}
}

func TestServerIdentityOmitsAnUnavailableCheckout(t *testing.T) {
	identity := ServerIdentity{
		StartedAt:    time.Now(),
		CheckoutRoot: "/checkout",
		Probe: func(string, buildinfo.Info, time.Time) (*buildinfo.Checkout, error) {
			return nil, nil
		},
	}

	status, body, _ := requestServer(t, NewRouterWithServer(fstest.MapFS{}, nil, nil, identity))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if _, ok := body["checkout"]; ok {
		t.Fatalf("an unavailable checkout was published: %s", body["checkout"])
	}
}

func TestServerIdentitySurvivesAProbeFailure(t *testing.T) {
	identity := ServerIdentity{
		StartedAt:    time.Now(),
		CheckoutRoot: "/checkout",
		Probe: func(string, buildinfo.Info, time.Time) (*buildinfo.Checkout, error) {
			return nil, errors.New("git is unavailable")
		},
	}

	status, body, _ := requestServer(t, NewRouterWithServer(fstest.MapFS{}, nil, nil, identity))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if _, ok := body["checkout"]; ok {
		t.Fatalf("a failed probe still published a checkout: %s", body["checkout"])
	}
	if got := rawString(t, body, "startedAt"); got == "" {
		t.Fatal("a failed probe dropped the process start time")
	}
}

func TestServerIdentityIsServedByTheDefaultRouter(t *testing.T) {
	status, body, _ := requestServer(t, NewRouter(fstest.MapFS{}))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if got := rawString(t, body, "service"); got != "specflow" {
		t.Fatalf("service = %q, want specflow", got)
	}
}

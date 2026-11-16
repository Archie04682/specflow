package api

import (
	"net/http"
	"time"

	"example.com/specflow/backend/internal/buildinfo"
)

// ServerIdentity is what the router can say about the running process. The
// zero value is valid and reports only the service name, which is what the
// router constructors that do not care about identity pass.
type ServerIdentity struct {
	// Info is the build stamp read when the process started.
	Info buildinfo.Info
	// StartedAt is when the process started serving.
	StartedAt time.Time
	// CheckoutRoot is the working directory the server was launched from; the
	// checkout probe walks up from it to find a SpecFlow source tree.
	CheckoutRoot string
	// Probe is a seam for tests. A nil probe uses buildinfo.ProbeCheckout.
	Probe ProbeFunc
}

// ProbeFunc reports the checkout at root. An error means the probe could not
// answer; the endpoint omits the report and stays successful either way.
type ProbeFunc func(root string, built buildinfo.Info, startedAt time.Time) (*buildinfo.Checkout, error)

// serverResponse is the wire shape of GET /api/server. Unknown values are
// omitted rather than sent empty or null, so a client can tell "not recorded"
// from "recorded as empty".
type serverResponse struct {
	Service      string            `json:"service"`
	Version      string            `json:"version,omitempty"`
	Revision     string            `json:"revision,omitempty"`
	RevisionTime string            `json:"revisionTime,omitempty"`
	Modified     *bool             `json:"modified,omitempty"`
	BuiltAt      string            `json:"builtAt,omitempty"`
	StartedAt    string            `json:"startedAt,omitempty"`
	Checkout     *checkoutResponse `json:"checkout,omitempty"`
}

type checkoutResponse struct {
	Revision     string `json:"revision,omitempty"`
	StaleReason  string `json:"staleReason,omitempty"`
	NewerSources int    `json:"newerSources,omitempty"`
}

// serverHandler reports the running build and, when the process was launched
// from a SpecFlow checkout, how that checkout relates to the build. Nothing
// here can fail a request: identity and checkout state are diagnostics.
func serverHandler(identity ServerIdentity) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, identity.response())
	}
}

func (identity ServerIdentity) response() serverResponse {
	payload := serverResponse{
		Service:   "specflow",
		Version:   identity.Info.Version,
		Revision:  identity.Info.Revision,
		Modified:  identity.Info.Modified,
		StartedAt: formatStamp(identity.StartedAt),
	}
	payload.RevisionTime = formatStamp(identity.Info.RevisionTime)
	payload.BuiltAt = formatStamp(identity.Info.BuiltAt)

	if checkout := identity.checkout(); checkout != nil {
		payload.Checkout = &checkoutResponse{
			Revision:     checkout.Revision,
			StaleReason:  checkout.StaleReason,
			NewerSources: checkout.NewerSources,
		}
	}
	return payload
}

// checkout runs the probe, treating a missing root, a probe error, and a
// non-checkout alike: the report is simply absent.
func (identity ServerIdentity) checkout() *buildinfo.Checkout {
	if identity.CheckoutRoot == "" {
		return nil
	}
	probe := identity.Probe
	if probe == nil {
		probe = func(root string, built buildinfo.Info, startedAt time.Time) (*buildinfo.Checkout, error) {
			return buildinfo.ProbeCheckout(root, built, startedAt), nil
		}
	}
	checkout, err := probe(identity.CheckoutRoot, identity.Info, identity.StartedAt)
	if err != nil {
		return nil
	}
	return checkout
}

// formatStamp renders a known time as RFC 3339 in UTC and an unknown one as an
// empty string, which the response omits.
func formatStamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

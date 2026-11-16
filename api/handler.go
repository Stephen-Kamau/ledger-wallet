package api

import (
	"net/http"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// Handler is the HTTP boundary over core.Ledger. core.Ledger is its ONLY domain
// dependency (10-api.md §4.3): the handler does transport and identity, everything
// financial happens behind the interface.
type Handler struct {
	ledger core.Ledger
	admins map[core.PrincipalID]bool
	// verify resolves a bearer token to a proven principal. It is the single
	// seam a real JWT/OIDC verifier would replace (§2.1); the toy default treats
	// the token's subject as the principal.
	verify func(token string) (core.PrincipalID, bool)
}

// Option configures a Handler.
type Option func(*Handler)

// WithTokens installs a static token→principal registry. An unknown token is
// rejected (401). Used by tests and by any deployment that wants a closed token
// set rather than the subject-is-the-token default.
func WithTokens(tokens map[string]core.PrincipalID) Option {
	return func(h *Handler) {
		reg := make(map[string]core.PrincipalID, len(tokens))
		for k, v := range tokens {
			reg[k] = v
		}
		h.verify = func(token string) (core.PrincipalID, bool) {
			p, ok := reg[token]
			return p, ok
		}
	}
}

// WithAdmins marks the principals allowed to set allow_negative on account
// creation (§2.4) and to read accounts they do not own (D-D1). Empty by default:
// the system/mint account is provisioned by seed/config, not an open endpoint.
func WithAdmins(admins ...core.PrincipalID) Option {
	return func(h *Handler) {
		for _, a := range admins {
			h.admins[a] = true
		}
	}
}

// NewHandler builds the API handler over a core.Ledger. By default the bearer
// token's subject IS the principal (any non-empty token authenticates as itself);
// pass WithTokens for a closed registry.
func NewHandler(ledger core.Ledger, opts ...Option) *Handler {
	h := &Handler{
		ledger: ledger,
		admins: map[core.PrincipalID]bool{},
		verify: subjectIsToken,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// subjectIsToken is the toy default verifier: the token subject is the principal.
// Any non-empty token authenticates as the principal of that same name. An empty
// token never reaches here (bearerToken rejects it first).
func subjectIsToken(token string) (core.PrincipalID, bool) {
	if token == "" {
		return "", false
	}
	return core.PrincipalID(token), true
}

// Router builds the stdlib ServeMux with the Go 1.22+ method+pattern routes and
// wraps it in the middleware chain (§4). /healthz is registered and exempt from
// auth; an unknown path returns a typed 404 (not ServeMux's plain-text default).
func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("POST /accounts", h.createAccount)
	mux.HandleFunc("GET /accounts", h.listAccounts) // D-C1
	mux.HandleFunc("GET /accounts/{id}", h.getBalance)
	mux.HandleFunc("POST /transfers", h.createTransfer)
	mux.HandleFunc("GET /accounts/{id}/statement", h.getStatement)
	mux.HandleFunc("POST /accounts/{id}/holds", h.placeHold)
	mux.HandleFunc("DELETE /accounts/{id}/holds/{holdId}", h.releaseHold)
	// Catch-all for unknown paths → typed 404. More specific patterns above win
	// (ServeMux longest-pattern precedence); a wrong method on a known path is
	// ServeMux's own 405.
	mux.HandleFunc("/", h.notFound)

	return chain(mux,
		recoverMW,
		requestIDMW,
		loggingMW,
		h.authMW,
	)
}

// isAllowedReader reports whether principal may read account owner's data (D-D1):
// the owner itself, or a configured admin (system/mint accounts).
func (h *Handler) isAllowedReader(principal, owner core.PrincipalID) bool {
	return principal == owner || h.admins[principal]
}

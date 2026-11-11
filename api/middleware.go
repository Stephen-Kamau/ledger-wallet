package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// This file is the middleware chain (10-api.md §4.2). Applied outermost → innermost:
//
//	recover → requestID → logging → auth → [handler]
//
// recover is outermost so it catches panics in everything below; requestID is next
// so every log line and error body (including a panic's 500) carries it; auth is
// innermost so a rejected request is cheap and never touches a handler.

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyPrincipal
)

// exemptFromAuth is the enumerated allowlist of paths that skip auth. It is an
// explicit set, never a `path != "/healthz"` skip (CLAUDE.md fail-closed rule:
// enumerate the exempt set, never key an exemption on !=).
var exemptFromAuth = map[string]bool{
	"/healthz": true,
}

// middleware is a standard http.Handler decorator.
type middleware func(http.Handler) http.Handler

// chain applies middlewares so the first listed is outermost.
func chain(h http.Handler, mws ...middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// recoverMW converts a panic into the typed 500 body (§3) and logs the request-id
// server-side. This is what guarantees "never a bare 500" even for an unexpected
// crash. It does not leak a stack to the client.
func recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("api: panic recovered (request_id=%s): %v", requestIDFrom(r.Context()), rec)
				writeErr(w, r, http.StatusInternalServerError, "internal", "an internal error occurred")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// requestIDMW generates or propagates X-Request-Id, stores it in ctx for handlers
// and the error writer, and echoes it as a response header.
func requestIDMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loggingMW writes one structured access line per request keyed by request-id.
func loggingMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("api: method=%s path=%s status=%d principal=%s duration=%s request_id=%s",
			r.Method, r.URL.Path, sw.status, principalFrom(r.Context()),
			time.Since(start), requestIDFrom(r.Context()))
	})
}

// authMW is the §2 boundary. It resolves the bearer token to a proven PrincipalID
// and nothing more: it does NOT load the account, compare owners, or pre-check R7.
// A missing/invalid token is 401 and never reaches a handler. /healthz is exempt.
func (h *Handler) authMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if exemptFromAuth[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		token, ok := bearerToken(r)
		if !ok {
			writeErr(w, r, http.StatusUnauthorized, "unauthenticated", "a bearer token is required")
			return
		}
		principal, ok := h.verify(token)
		if !ok {
			writeErr(w, r, http.StatusUnauthorized, "unauthenticated", "the bearer token is not valid")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyPrincipal, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerToken extracts the token from an `Authorization: Bearer <token>` header.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// statusWriter captures the status code the handler wrote for the access log.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(b)
}

func newRequestID() string {
	var buf [12]byte
	_, _ = rand.Read(buf[:])
	return "req_" + hex.EncodeToString(buf[:])
}

func requestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

func principalFrom(ctx context.Context) core.PrincipalID {
	if v, ok := ctx.Value(ctxKeyPrincipal).(core.PrincipalID); ok {
		return v
	}
	return ""
}

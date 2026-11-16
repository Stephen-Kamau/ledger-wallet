package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// This file owns the single JSON encoder and the one typed error → HTTP status
// map (10-api.md §3, R13). There is exactly one place that serialises the error
// body, so a 400, a 409 and a 500 are byte-shape identical: a client never has to
// branch on status to find the code.

// errorBody is the one error shape, every error (§3).
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// coreErrorRow binds a frozen core sentinel to its HTTP status and stable code.
// Matched with errors.Is, so a wrapped LedgerError of the same Code still maps.
type coreErrorRow struct {
	sentinel error
	status   int
	code     string
}

// coreErrorMap is the fail-closed contract: EVERY exported core.Err* sentinel has
// a row here. A new core sentinel with no row falls through to the typed 500
// fallback (never bare), and api_test.go asserts every sentinel is present so the
// gap is a failing test, not a silent 500.
var coreErrorMap = []coreErrorRow{
	{core.ErrUnbalanced, http.StatusUnprocessableEntity, "unbalanced"},              // 422 (R1)
	{core.ErrCurrencyMismatch, http.StatusUnprocessableEntity, "currency_mismatch"}, // 422 (R8)
	{core.ErrInsufficientFunds, http.StatusConflict, "insufficient_funds"},          // 409 (R3)
	{core.ErrUnauthorized, http.StatusForbidden, "unauthorized"},                    // 403 (R7)
	{core.ErrAccountNotFound, http.StatusNotFound, "account_not_found"},             // 404
	{core.ErrIdempotencyConflict, http.StatusConflict, "idempotency_conflict"},      // 409 (R4)
	{core.ErrInvalidAccount, http.StatusUnprocessableEntity, "invalid_account"},     // 422 (R6)
	{core.ErrHoldNotFound, http.StatusNotFound, "hold_not_found"},                   // 404 (holds)
}

// writeJSON serialises a success body. A marshal failure is the one place a bare
// 500 could leak; it is logged and a typed internal error is written instead.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	buf, err := json.Marshal(body)
	if err != nil {
		log.Printf("api: response marshal failed (request_id=%s): %v", requestIDFrom(r.Context()), err)
		writeErr(w, r, http.StatusInternalServerError, "internal", "response could not be encoded")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

// writeErr serialises the one §3 error body. It is the single error writer; the
// recover, router and handler paths all funnel through here so even a panic's 500
// carries the typed body and the request-id.
func writeErr(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	body := errorBody{Error: errorDetail{
		Code:      code,
		Message:   message,
		RequestID: requestIDFrom(r.Context()),
	}}
	buf, err := json.Marshal(body)
	if err != nil {
		// Last-resort: the error body itself failed to marshal. Write a minimal
		// typed body by hand so the response is still never bare.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"error encoding failed","request_id":""}}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

// writeCoreErr is the single translation point for a core error (§3). It walks the
// map with errors.Is; an unmapped error hits the typed 500 fallback, logged with
// the request-id. The fallback is never bare.
func writeCoreErr(w http.ResponseWriter, r *http.Request, err error) {
	for _, row := range coreErrorMap {
		if errors.Is(err, row.sentinel) {
			writeErr(w, r, row.status, row.code, messageFor(err, row))
			return
		}
	}
	log.Printf("api: unmapped error (request_id=%s): %v", requestIDFrom(r.Context()), err)
	writeErr(w, r, http.StatusInternalServerError, "internal", "an internal error occurred")
}

// messageFor prefers the live error's own message (a LedgerError carries one) over
// the row's code, so the body shows the domain detail.
func messageFor(err error, row coreErrorRow) string {
	var le *core.LedgerError
	if errors.As(err, &le) && le.Message != "" {
		return le.Message
	}
	return row.code
}

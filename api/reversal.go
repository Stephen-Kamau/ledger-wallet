package api

import (
	"net/http"
)

// This file is the reversal endpoint: POST /entries/{id}/reverse. It is the HTTP
// face of core.Ledger.Reverse (Amara's), and like every other handler it is a thin
// translation - authenticate (middleware) → read the idempotency key → call the
// method → encode the compensating entry or map the typed error. No financial logic
// lives here: a reversal is a compensating INSERT inside core, never an UPDATE of a
// posting (R5, append-only).
//
// Auth boundary. D-D1 settles the READ side: a read of an account the principal does
// not own is 404, not 403, so account existence does not leak. D-D1 is scoped to
// balance and statement reads ("reads are 404 or 200") and this endpoint is a WRITE,
// so it follows the write convention the rest of the surface already uses - core
// enforces ownership (R7, reused) and returns ErrUnauthorized, which the one error
// map renders as 403, exactly as POST /transfers and POST /accounts/{id}/holds do.
// The api layer does NOT pre-check ownership here; that check belongs behind the
// interface.

// reverseEntry: POST /entries/{id}/reverse → Ledger.Reverse. The entry id is the
// path param; the Idempotency-Key header is required, as it is for every other
// write. A replay returns the original compensating entry with 200 and no second
// posting; a first write is 201. An entry that is already reversed is a typed 409
// (already_reversed) through coreErrorMap - never a bare 500 (R13).
func (h *Handler) reverseEntry(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())
	entryID := r.PathValue("id")

	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeErr(w, r, http.StatusBadRequest, "idempotency_key_required",
			"Idempotency-Key header is required")
		return
	}
	// A reversal carries no body: the entry is the path param and the key is a
	// header. A body is accepted and ignored by stdlib, so nothing to decode.

	// Positional, matching ReleaseHold. The authorizer is the ONLY thing the API
	// asserts about identity; core checks that it owns the account the compensating
	// entry DEBITS - the original's DESTINATION, since that is where the money went.
	res, err := h.ledger.Reverse(r.Context(), principal, entryID, key)
	if err != nil {
		writeCoreErr(w, r, err)
		return
	}

	status := http.StatusCreated
	if res.Replayed {
		status = http.StatusOK // a replay is a success, not an error (R4)
	}
	writeJSON(w, r, status, toReversalResp(entryID, res.Transfer, res.Replayed))
}

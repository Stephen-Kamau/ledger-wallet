package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// Reversal is driven through fakeLedger rather than the real engine: the financial
// semantics of a reversal (the compensating postings, the already-reversed guard)
// are core's invariants and core's tests. What api owes is the translation - the
// path param reaches core as the entry, the header reaches it as the idempotency
// key, the status is 201/200 on the replay axis, and every typed error maps to the
// documented status through the one fail-closed map (R13).

// reverseFake returns a handler over a programmable ledger plus the fake itself, so
// a test can assert what the handler passed through the interface.
func reverseFake(result core.TransferResult, err error) (*Handler, *fakeLedger) {
	f := &fakeLedger{reverseResult: result, reverseErr: err}
	return NewHandler(f), f
}

// compensating is a stand-in compensating transfer as core would return it.
func compensating() core.Transfer {
	return core.Transfer{
		ID:        "xfer_rev",
		Owner:     "alice",
		From:      "acct_b",
		To:        "acct_a",
		Amount:    100,
		Currency:  "USD",
		CreatedAt: time.Unix(0, 0).UTC(),
		Postings: []core.Posting{
			{Account: "acct_b", Amount: -100, Seq: 2},
			{Account: "acct_a", Amount: 100, Seq: 2},
		},
	}
}

// A first reversal is 201 and returns the compensating entry, and the handler hands
// core the path's entry id, the header's key and the authenticated principal.
func TestReverse_201_ReturnsCompensatingEntry(t *testing.T) {
	h, f := reverseFake(core.TransferResult{Transfer: compensating()}, nil)

	rec := do(t, h, http.MethodPost, "/entries/post_1/reverse", "alice", nil,
		map[string]string{"Idempotency-Key": "rev-1"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	var resp reversalResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ReversesEntryID != "post_1" {
		t.Fatalf("reverses_entry_id = %q, want post_1", resp.ReversesEntryID)
	}
	if resp.TransferID != "xfer_rev" || resp.Amount != 100 {
		t.Fatalf("unexpected compensating entry: %+v", resp)
	}
	if resp.Replayed {
		t.Fatalf("a first reversal must not report replayed")
	}
	if n := len(resp.Postings); n != 2 {
		t.Fatalf("compensating entry must carry both legs, got %d", n)
	}

	// The translation itself: nothing about identity comes from the body.
	if f.reverseEntryID != "post_1" {
		t.Errorf("core got entry %q, want post_1", f.reverseEntryID)
	}
	if f.reverseKey != "rev-1" {
		t.Errorf("core got key %q, want rev-1", f.reverseKey)
	}
	if f.reverseAuthorizer != "alice" {
		t.Errorf("core got authorizer %q, want the authenticated principal alice", f.reverseAuthorizer)
	}
}

// A replayed reversal is a success, not an error: 200 with replayed set (R4).
func TestReverse_Replay_200(t *testing.T) {
	h, _ := reverseFake(core.TransferResult{Transfer: compensating(), Replayed: true}, nil)

	rec := do(t, h, http.MethodPost, "/entries/post_1/reverse", "alice", nil,
		map[string]string{"Idempotency-Key": "rev-1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 on replay, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp reversalResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Replayed {
		t.Fatalf("a replay must report replayed=true")
	}
}

// The Idempotency-Key header is required, exactly as it is for transfers and holds.
// This is checked before core is reached, so a keyless reversal writes nothing.
func TestReverse_MissingIdempotencyKey_400(t *testing.T) {
	h, f := reverseFake(core.TransferResult{Transfer: compensating()}, nil)

	rec := do(t, h, http.MethodPost, "/entries/post_1/reverse", "alice", nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	if code := decodeErr(t, rec).Code; code != "idempotency_key_required" {
		t.Fatalf("want idempotency_key_required, got %q", code)
	}
	if f.reverseCalled {
		t.Fatalf("core must not be reached without an idempotency key")
	}
}

// Reversal is behind the same auth boundary as every other non-exempt path.
func TestReverse_NoBearer_401(t *testing.T) {
	h, f := reverseFake(core.TransferResult{Transfer: compensating()}, nil)

	rec := do(t, h, http.MethodPost, "/entries/post_1/reverse", "", nil,
		map[string]string{"Idempotency-Key": "rev-1"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d (%s)", rec.Code, rec.Body.String())
	}
	if f.reverseCalled {
		t.Fatalf("an unauthenticated request must never reach core")
	}
}

// Every typed core error the reversal path can raise maps to its documented status
// and stable code - never a bare 500 for a known rule (R13).
//
// The ownership row is the auth-boundary decision this endpoint rests on. D-D1
// settles the READ side (a non-owned account reads 404, not 403, so existence does
// not leak) and is scoped to balance and statement reads. A reversal is a WRITE, so
// it keeps the write convention already used by POST /transfers and POST
// /accounts/{id}/holds: core enforces R7 and api renders ErrUnauthorized as 403.
func TestReverse_ErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"already reversed", core.ErrAlreadyReversed, http.StatusConflict, "already_reversed"},
		{"unknown entry", core.ErrEntryNotFound, http.StatusNotFound, "entry_not_found"},
		{"not the owner", core.ErrUnauthorized, http.StatusForbidden, "unauthorized"},
		{"key reused", core.ErrIdempotencyConflict, http.StatusConflict, "idempotency_conflict"},
		{"account gone", core.ErrAccountNotFound, http.StatusNotFound, "account_not_found"},
		// Reachable because the compensating entry DEBITS the original's
		// destination: that account may already have spent the money (R3).
		{"destination spent it", core.ErrInsufficientFunds, http.StatusConflict, "insufficient_funds"},
		{"original unbalanced", core.ErrUnbalanced, http.StatusUnprocessableEntity, "unbalanced"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := reverseFake(core.TransferResult{}, tc.err)
			rec := do(t, h, http.MethodPost, "/entries/post_1/reverse", "alice", nil,
				map[string]string{"Idempotency-Key": "rev-1"})
			if rec.Code != tc.wantStatus {
				t.Fatalf("want %d, got %d (%s)", tc.wantStatus, rec.Code, rec.Body.String())
			}
			if code := decodeErr(t, rec).Code; code != tc.wantCode {
				t.Fatalf("want code %q, got %q", tc.wantCode, code)
			}
		})
	}
}

// An unmapped error still carries the typed body, never a bare 500 (R13).
func TestReverse_UnmappedError_Typed500(t *testing.T) {
	h, _ := reverseFake(core.TransferResult{}, &core.LedgerError{Code: "not_in_the_map", Message: "x"})
	rec := do(t, h, http.MethodPost, "/entries/post_1/reverse", "alice", nil,
		map[string]string{"Idempotency-Key": "rev-1"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "internal" {
		t.Fatalf("want typed internal body, got %q", code)
	}
}

// GET on the reversal path is not a route: reversal is POST only.
func TestReverse_WrongMethod_NotOK(t *testing.T) {
	h, f := reverseFake(core.TransferResult{Transfer: compensating()}, nil)
	rec := do(t, h, http.MethodGet, "/entries/post_1/reverse", "alice", nil,
		map[string]string{"Idempotency-Key": "rev-1"})
	if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
		t.Fatalf("GET must not reverse an entry, got %d", rec.Code)
	}
	if f.reverseCalled {
		t.Fatalf("GET must never reach Reverse")
	}
}

// The reversal body's JSON keys ARE the cross-stream contract: Nia's web layer
// codes to `ReversalBody` in web/src/api/types.ts. A rename here is a silent break
// over there (TypeScript cannot see this struct), so the keys are pinned exactly.
// Extra keys are tolerated by a structural TS consumer; a MISSING one is not.
func TestReverse_WireContract_Keys(t *testing.T) {
	h, _ := reverseFake(core.TransferResult{Transfer: compensating()}, nil)
	rec := do(t, h, http.MethodPost, "/entries/post_1/reverse", "alice", nil,
		map[string]string{"Idempotency-Key": "rev-1"})

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{
		"transfer_id", "reverses_entry_id", "amount",
		"currency", "replayed", "created_at", "postings",
	} {
		if _, ok := got[key]; !ok {
			t.Errorf("ReversalBody key %q missing from the response: %s", key, rec.Body.String())
		}
	}
}

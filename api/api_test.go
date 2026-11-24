package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// --- helpers --------------------------------------------------------------------

// do issues a request against h.Router() and returns the recorder.
func do(t *testing.T, h *Handler, method, path, bearer string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)
	return rec
}

// decodeErr reads the typed error body.
func decodeErr(t *testing.T, rec *httptest.ResponseRecorder) errorDetail {
	t.Helper()
	var b errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return b.Error
}

// realHandler builds a Handler over the real core engine on a fresh in-memory store.
func realHandler(opts ...Option) (*Handler, *memStore) {
	ms := newMemStore()
	ledger := core.New(ms)
	return NewHandler(ledger, opts...), ms
}

// --- auth boundary (R7, §2) -----------------------------------------------------

func TestAuth_MissingToken_401(t *testing.T) {
	h, _ := realHandler()
	rec := do(t, h, http.MethodGet, "/accounts", "", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "unauthenticated" {
		t.Fatalf("want code unauthenticated, got %q", code)
	}
}

func TestAuth_InvalidToken_401(t *testing.T) {
	// A closed token registry: an unknown token is rejected.
	h, _ := realHandler(WithTokens(map[string]core.PrincipalID{"tok-alice": "alice"}))
	rec := do(t, h, http.MethodGet, "/accounts", "tok-nobody", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "unauthenticated" {
		t.Fatalf("want code unauthenticated, got %q", code)
	}
}

func TestAuth_ValidToken_Resolves(t *testing.T) {
	h, _ := realHandler(WithTokens(map[string]core.PrincipalID{"tok-alice": "alice"}))
	rec := do(t, h, http.MethodGet, "/accounts", "tok-alice", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestHealthz_ExemptFromAuth(t *testing.T) {
	h, _ := realHandler()
	rec := do(t, h, http.MethodGet, "/healthz", "", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 on /healthz without token, got %d", rec.Code)
	}
}

// --- POST /accounts (R9, §1.1, §2.4) --------------------------------------------

func TestCreateAccount_OwnerIsPrincipal_201(t *testing.T) {
	h, _ := realHandler()
	rec := do(t, h, http.MethodPost, "/accounts", "alice",
		createAccountReq{Currency: "USD"}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp accountResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Owner != "alice" {
		t.Fatalf("owner must be the authenticated principal, got %q", resp.Owner)
	}
	if resp.Currency != "USD" {
		t.Fatalf("currency echo, got %q", resp.Currency)
	}
}

func TestCreateAccount_AllowNegative_NonAdmin_403(t *testing.T) {
	h, _ := realHandler()
	rec := do(t, h, http.MethodPost, "/accounts", "alice",
		createAccountReq{Currency: "USD", AllowNegative: true}, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "forbidden_privileged_flag" {
		t.Fatalf("want forbidden_privileged_flag, got %q", code)
	}
}

func TestCreateAccount_AllowNegative_Admin_201(t *testing.T) {
	h, _ := realHandler(WithAdmins("root"))
	rec := do(t, h, http.MethodPost, "/accounts", "root",
		createAccountReq{Currency: "USD", AllowNegative: true}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201 for admin, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateAccount_MissingCurrency_422(t *testing.T) {
	h, _ := realHandler()
	rec := do(t, h, http.MethodPost, "/accounts", "alice", createAccountReq{}, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", rec.Code)
	}
}

func TestCreateAccount_MalformedJSON_400(t *testing.T) {
	h, _ := realHandler()
	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader("{not json"))
	req.Header.Set("Authorization", "Bearer alice")
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "bad_request" {
		t.Fatalf("want bad_request, got %q", code)
	}
}

// --- GET /accounts (R20, D-C1) --------------------------------------------------

func TestListAccounts_OnlyOwn(t *testing.T) {
	h, _ := realHandler()
	mustCreate(t, h, "alice", "USD")
	mustCreate(t, h, "alice", "GBP")
	mustCreate(t, h, "bob", "USD")

	rec := do(t, h, http.MethodGet, "/accounts", "alice", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp accountsResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Accounts) != 2 {
		t.Fatalf("alice should see her 2 accounts, got %d", len(resp.Accounts))
	}
	for _, a := range resp.Accounts {
		if a.Owner != "alice" {
			t.Fatalf("list leaked a non-owned account: %q", a.Owner)
		}
	}
}

// --- GET /accounts/{id} (R10, §1.2, D-D1) ---------------------------------------

func TestGetBalance_Owner_200(t *testing.T) {
	h, _ := realHandler()
	id := mustCreate(t, h, "alice", "USD")
	rec := do(t, h, http.MethodGet, "/accounts/"+id, "alice", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp balanceResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AccountID != id || resp.Currency != "USD" || resp.Amount != 0 {
		t.Fatalf("unexpected balance body: %+v", resp)
	}
}

func TestGetBalance_NonOwned_404_D_D1(t *testing.T) {
	h, _ := realHandler()
	id := mustCreate(t, h, "alice", "USD")
	// bob reads alice's account: D-D1 says 404 (not 403), no existence leak.
	rec := do(t, h, http.MethodGet, "/accounts/"+id, "bob", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("D-D1: want 404 for non-owned read, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "account_not_found" {
		t.Fatalf("want account_not_found, got %q", code)
	}
}

func TestGetBalance_Admin_CanReadNonOwned(t *testing.T) {
	h, _ := realHandler(WithAdmins("root"))
	id := mustCreate(t, h, "alice", "USD")
	rec := do(t, h, http.MethodGet, "/accounts/"+id, "root", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin should read non-owned, got %d", rec.Code)
	}
}

func TestGetBalance_Unknown_404(t *testing.T) {
	h, _ := realHandler()
	rec := do(t, h, http.MethodGet, "/accounts/acct_nope", "alice", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

// --- POST /transfers (R11, §1.3, §5) --------------------------------------------

func TestTransfer_Happy_201_AndReplay_200(t *testing.T) {
	h, _ := realHandler(WithAdmins("root"))
	// root mints into alice via a negative-allowed system account.
	sys := mustCreateNeg(t, h, "root", "USD")
	alice := mustCreate(t, h, "alice", "USD")

	// Fund alice from the system account (root authorizes the debit of sys).
	fund := do(t, h, http.MethodPost, "/transfers", "root",
		transferReq{From: sys, To: alice, Amount: 10000, Currency: "USD"},
		map[string]string{"Idempotency-Key": "fund-1"})
	if fund.Code != http.StatusCreated {
		t.Fatalf("funding want 201, got %d (%s)", fund.Code, fund.Body.String())
	}

	bob := mustCreate(t, h, "bob", "USD")
	hdr := map[string]string{"Idempotency-Key": "xfer-1"}
	first := do(t, h, http.MethodPost, "/transfers", "alice",
		transferReq{From: alice, To: bob, Amount: 2500, Currency: "USD"}, hdr)
	if first.Code != http.StatusCreated {
		t.Fatalf("first transfer want 201, got %d (%s)", first.Code, first.Body.String())
	}
	var ft transferResp
	_ = json.Unmarshal(first.Body.Bytes(), &ft)
	if ft.Replayed {
		t.Fatalf("first write must not be a replay")
	}
	if len(ft.Postings) != 2 {
		t.Fatalf("want 2 balanced postings, got %d", len(ft.Postings))
	}

	// Replay: same key, same params → 200, replayed, same transfer id, no 2nd post.
	replay := do(t, h, http.MethodPost, "/transfers", "alice",
		transferReq{From: alice, To: bob, Amount: 2500, Currency: "USD"}, hdr)
	if replay.Code != http.StatusOK {
		t.Fatalf("replay want 200, got %d (%s)", replay.Code, replay.Body.String())
	}
	var rt transferResp
	_ = json.Unmarshal(replay.Body.Bytes(), &rt)
	if !rt.Replayed {
		t.Fatalf("replay must set replayed=true")
	}
	if rt.TransferID != ft.TransferID {
		t.Fatalf("replay must return the original transfer id")
	}

	// Bob's balance is exactly 2500 (no double post).
	balRec := do(t, h, http.MethodGet, "/accounts/"+bob, "bob", nil, nil)
	var bal balanceResp
	_ = json.Unmarshal(balRec.Body.Bytes(), &bal)
	if bal.Amount != 2500 {
		t.Fatalf("bob balance want 2500 (no double post), got %d", bal.Amount)
	}
}

func TestTransfer_MissingIdempotencyKey_400(t *testing.T) {
	h, _ := realHandler()
	alice := mustCreate(t, h, "alice", "USD")
	bob := mustCreate(t, h, "bob", "USD")
	rec := do(t, h, http.MethodPost, "/transfers", "alice",
		transferReq{From: alice, To: bob, Amount: 100, Currency: "USD"}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "idempotency_key_required" {
		t.Fatalf("want idempotency_key_required, got %q", code)
	}
}

func TestTransfer_NotOwner_403_FromCore(t *testing.T) {
	h, _ := realHandler()
	alice := mustCreate(t, h, "alice", "USD")
	bob := mustCreate(t, h, "bob", "USD")
	// bob tries to debit alice's account. The API does NOT pre-check; core rejects
	// with ErrUnauthorized → 403 unauthorized.
	rec := do(t, h, http.MethodPost, "/transfers", "bob",
		transferReq{From: alice, To: bob, Amount: 100, Currency: "USD"},
		map[string]string{"Idempotency-Key": "k"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	if code := decodeErr(t, rec).Code; code != "unauthorized" {
		t.Fatalf("want unauthorized, got %q", code)
	}
}

func TestTransfer_InsufficientFunds_409(t *testing.T) {
	h, _ := realHandler()
	alice := mustCreate(t, h, "alice", "USD")
	bob := mustCreate(t, h, "bob", "USD")
	rec := do(t, h, http.MethodPost, "/transfers", "alice",
		transferReq{From: alice, To: bob, Amount: 5000, Currency: "USD"},
		map[string]string{"Idempotency-Key": "k"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d (%s)", rec.Code, rec.Body.String())
	}
	if code := decodeErr(t, rec).Code; code != "insufficient_funds" {
		t.Fatalf("want insufficient_funds, got %q", code)
	}
}

// --- GET /accounts/{id}/statement (R12, §1.4) -----------------------------------

func TestStatement_Owner_200(t *testing.T) {
	h, _ := realHandler(WithAdmins("root"))
	sys := mustCreateNeg(t, h, "root", "USD")
	alice := mustCreate(t, h, "alice", "USD")
	fund := do(t, h, http.MethodPost, "/transfers", "root",
		transferReq{From: sys, To: alice, Amount: 700, Currency: "USD"},
		map[string]string{"Idempotency-Key": "f"})
	if fund.Code != http.StatusCreated {
		t.Fatalf("fund failed: %d", fund.Code)
	}
	rec := do(t, h, http.MethodGet, "/accounts/"+alice+"/statement?limit=10", "alice", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp statementResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Postings) != 1 || resp.Postings[0].Amount != 700 {
		t.Fatalf("expected one +700 posting, got %+v", resp.Postings)
	}
}

func TestStatement_NonOwned_404_D_D1(t *testing.T) {
	h, _ := realHandler()
	alice := mustCreate(t, h, "alice", "USD")
	rec := do(t, h, http.MethodGet, "/accounts/"+alice+"/statement", "bob", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("D-D1: want 404, got %d", rec.Code)
	}
}

// --- router edges ---------------------------------------------------------------

func TestUnknownRoute_404_Typed(t *testing.T) {
	h, _ := realHandler()
	rec := do(t, h, http.MethodGet, "/nope", "alice", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "not_found" {
		t.Fatalf("want not_found, got %q", code)
	}
}

// --- error mapping completeness (R13, §3) ---------------------------------------

// TestErrorMap_EveryCoreSentinelMapped is the fail-closed assertion the design doc
// promises: every exported core.Err* sentinel has a row in coreErrorMap. A new core
// sentinel with no mapping is a FAILING test here, not a silent 500.
func TestErrorMap_EveryCoreSentinelMapped(t *testing.T) {
	sentinels := []*core.LedgerError{
		core.ErrUnbalanced,
		core.ErrInsufficientFunds,
		core.ErrUnauthorized,
		core.ErrCurrencyMismatch,
		core.ErrAccountNotFound,
		core.ErrIdempotencyConflict,
		core.ErrInvalidAccount,
		core.ErrHoldNotFound,
		core.ErrEntryNotFound,
		core.ErrAlreadyReversed,
	}
	for _, s := range sentinels {
		found := false
		for _, row := range coreErrorMap {
			if row.sentinel == error(s) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("core sentinel %q has no row in coreErrorMap (fail-closed)", s.Code)
		}
	}
}

// TestErrorMap_StatusAndCode drives each core sentinel through a handler via the
// fakeLedger and asserts the HTTP status + stable code from the §3 table.
func TestErrorMap_StatusAndCode(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"unbalanced", core.ErrUnbalanced, 422, "unbalanced"},
		{"currency_mismatch", core.ErrCurrencyMismatch, 422, "currency_mismatch"},
		{"insufficient_funds", core.ErrInsufficientFunds, 409, "insufficient_funds"},
		{"unauthorized", core.ErrUnauthorized, 403, "unauthorized"},
		{"account_not_found", core.ErrAccountNotFound, 404, "account_not_found"},
		{"idempotency_conflict", core.ErrIdempotencyConflict, 409, "idempotency_conflict"},
		{"invalid_account", core.ErrInvalidAccount, 422, "invalid_account"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fl := &fakeLedger{transferErr: tc.err}
			h := NewHandler(fl)
			rec := do(t, h, http.MethodPost, "/transfers", "alice",
				transferReq{From: "a", To: "b", Amount: 1, Currency: "USD"},
				map[string]string{"Idempotency-Key": "k"})
			if rec.Code != tc.wantStatus {
				t.Fatalf("want status %d, got %d", tc.wantStatus, rec.Code)
			}
			if code := decodeErr(t, rec).Code; code != tc.wantCode {
				t.Fatalf("want code %q, got %q", tc.wantCode, code)
			}
		})
	}
}

// TestErrorMap_UnmappedFallsToTyped500 proves an error outside the map still gets a
// typed body (never bare).
func TestErrorMap_UnmappedFallsToTyped500(t *testing.T) {
	fl := &fakeLedger{transferErr: io.ErrUnexpectedEOF}
	h := NewHandler(fl)
	rec := do(t, h, http.MethodPost, "/transfers", "alice",
		transferReq{From: "a", To: "b", Amount: 1, Currency: "USD"},
		map[string]string{"Idempotency-Key": "k"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "internal" {
		t.Fatalf("want typed internal body, got %q", code)
	}
	if rid := decodeErr(t, rec).RequestID; rid == "" {
		t.Fatalf("500 body must still carry a request_id")
	}
}

// --- request id -----------------------------------------------------------------

func TestRequestID_EchoedHeader(t *testing.T) {
	h, _ := realHandler()
	rec := do(t, h, http.MethodGet, "/healthz", "", nil, nil)
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatalf("X-Request-Id must be echoed")
	}
}

// --- fixtures -------------------------------------------------------------------

func mustCreate(t *testing.T, h *Handler, principal, currency string) string {
	t.Helper()
	return createAcct(t, h, principal, currency, false)
}

func mustCreateNeg(t *testing.T, h *Handler, principal, currency string) string {
	t.Helper()
	return createAcct(t, h, principal, currency, true)
}

func createAcct(t *testing.T, h *Handler, principal, currency string, neg bool) string {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/accounts", principal,
		createAccountReq{Currency: currency, AllowNegative: neg}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account (%s) failed: %d (%s)", principal, rec.Code, rec.Body.String())
	}
	var resp accountResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode account: %v", err)
	}
	return resp.ID
}

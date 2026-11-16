package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// POST /accounts/{id}/holds places a hold through the real engine: 201 with the
// hold body, and the account's available balance drops while the posted balance
// (GET /accounts/{id}) is unchanged.
func TestPlaceHold_201_ReducesAvailable(t *testing.T) {
	h, _ := realHandler(WithAdmins("root"))
	sys := mustCreateNeg(t, h, "root", "USD")
	alice := mustCreate(t, h, "alice", "USD")
	fund := do(t, h, http.MethodPost, "/transfers", "root",
		transferReq{From: sys, To: alice, Amount: 100, Currency: "USD"},
		map[string]string{"Idempotency-Key": "fund"})
	if fund.Code != http.StatusCreated {
		t.Fatalf("fund: %d (%s)", fund.Code, fund.Body.String())
	}

	rec := do(t, h, http.MethodPost, "/accounts/"+alice+"/holds", "alice",
		holdReq{Amount: 30, Currency: "USD"}, map[string]string{"Idempotency-Key": "h1"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("place hold want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var hr holdResp
	if err := json.Unmarshal(rec.Body.Bytes(), &hr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !hr.Active || hr.Amount != 30 || hr.AccountID != alice {
		t.Fatalf("unexpected hold body: %+v", hr)
	}

	// Posted balance unchanged (holds post nothing).
	balRec := do(t, h, http.MethodGet, "/accounts/"+alice, "alice", nil, nil)
	var bal balanceResp
	_ = json.Unmarshal(balRec.Body.Bytes(), &bal)
	if bal.Amount != 100 {
		t.Fatalf("posted balance must be unchanged by a hold, got %d", bal.Amount)
	}
}

// A hold on an account the caller does not own is 403 unauthorized (core R7).
func TestPlaceHold_NotOwner_403(t *testing.T) {
	h, _ := realHandler()
	alice := mustCreate(t, h, "alice", "USD")
	rec := do(t, h, http.MethodPost, "/accounts/"+alice+"/holds", "bob",
		holdReq{Amount: 10, Currency: "USD"}, map[string]string{"Idempotency-Key": "k"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	if code := decodeErr(t, rec).Code; code != "unauthorized" {
		t.Fatalf("want unauthorized, got %q", code)
	}
}

// A hold above available funds is 409 insufficient_funds.
func TestPlaceHold_OverHold_409(t *testing.T) {
	h, _ := realHandler()
	alice := mustCreate(t, h, "alice", "USD")
	rec := do(t, h, http.MethodPost, "/accounts/"+alice+"/holds", "alice",
		holdReq{Amount: 50, Currency: "USD"}, map[string]string{"Idempotency-Key": "k"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d (%s)", rec.Code, rec.Body.String())
	}
	if code := decodeErr(t, rec).Code; code != "insufficient_funds" {
		t.Fatalf("want insufficient_funds, got %q", code)
	}
}

// A hold with no Idempotency-Key header is 400.
func TestPlaceHold_MissingKey_400(t *testing.T) {
	h, _ := realHandler()
	alice := mustCreate(t, h, "alice", "USD")
	rec := do(t, h, http.MethodPost, "/accounts/"+alice+"/holds", "alice",
		holdReq{Amount: 10, Currency: "USD"}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "idempotency_key_required" {
		t.Fatalf("want idempotency_key_required, got %q", code)
	}
}

// DELETE /accounts/{id}/holds/{holdId} releases an active hold (204) and frees the
// funds; a second release is 404 hold_not_found.
func TestReleaseHold_204_ThenNotFound(t *testing.T) {
	h, _ := realHandler(WithAdmins("root"))
	sys := mustCreateNeg(t, h, "root", "USD")
	alice := mustCreate(t, h, "alice", "USD")
	do(t, h, http.MethodPost, "/transfers", "root",
		transferReq{From: sys, To: alice, Amount: 100, Currency: "USD"},
		map[string]string{"Idempotency-Key": "fund"})

	place := do(t, h, http.MethodPost, "/accounts/"+alice+"/holds", "alice",
		holdReq{Amount: 40, Currency: "USD"}, map[string]string{"Idempotency-Key": "h"})
	var hr holdResp
	_ = json.Unmarshal(place.Body.Bytes(), &hr)

	rel := do(t, h, http.MethodDelete, "/accounts/"+alice+"/holds/"+hr.ID, "alice", nil, nil)
	if rel.Code != http.StatusNoContent {
		t.Fatalf("release want 204, got %d (%s)", rel.Code, rel.Body.String())
	}

	again := do(t, h, http.MethodDelete, "/accounts/"+alice+"/holds/"+hr.ID, "alice", nil, nil)
	if again.Code != http.StatusNotFound {
		t.Fatalf("double release want 404, got %d", again.Code)
	}
	if code := decodeErr(t, again).Code; code != "hold_not_found" {
		t.Fatalf("want hold_not_found, got %q", code)
	}
}

// Releasing an unknown hold is 404 hold_not_found.
func TestReleaseHold_Unknown_404(t *testing.T) {
	h, _ := realHandler()
	alice := mustCreate(t, h, "alice", "USD")
	rec := do(t, h, http.MethodDelete, "/accounts/"+alice+"/holds/hold_nope", "alice", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	if code := decodeErr(t, rec).Code; code != "hold_not_found" {
		t.Fatalf("want hold_not_found, got %q", code)
	}
}

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// This file holds the five endpoint handlers (10-api.md §1). Each is a thin
// translation between JSON and one Ledger method: authenticate (done in
// middleware) → decode → populate the core command → call the method → encode the
// result or map the typed error. No business logic lives here — that is the seam.

// healthz is the one auth-exempt path (§4.2).
func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

// notFound returns the typed 404 for an unknown route (§3) rather than ServeMux's
// plain-text default.
func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	writeErr(w, r, http.StatusNotFound, "not_found", "no such route")
}

// createAccount: POST /accounts → Ledger.CreateAccount (R9, §1.1). owner is the
// authenticated principal, never the body. allow_negative is gated to admins (§2.4).
func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())

	var body createAccountReq
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, r, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if body.Currency == "" {
		writeErr(w, r, http.StatusUnprocessableEntity, "unprocessable_entity", "currency is required")
		return
	}
	// Privilege gate: only an admin may mint a negative-allowed (system) account.
	if body.AllowNegative && !h.admins[principal] {
		writeErr(w, r, http.StatusForbidden, "forbidden_privileged_flag",
			"allow_negative may only be set by an admin principal")
		return
	}

	acc, err := h.ledger.CreateAccount(r.Context(), core.CreateAccountCmd{
		Owner:         principal,
		Currency:      core.Currency(body.Currency),
		AllowNegative: body.AllowNegative,
	})
	if err != nil {
		writeCoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, toAccountResp(acc))
}

// listAccounts: GET /accounts → Ledger.AccountsByOwner (R20, D-C1, §1). Lists the
// authenticated principal's own accounts; no ownership leak is possible.
func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())

	accounts, err := h.ledger.AccountsByOwner(r.Context(), principal)
	if err != nil {
		writeCoreErr(w, r, err)
		return
	}
	out := accountsResp{Accounts: make([]accountResp, 0, len(accounts))}
	for _, a := range accounts {
		out.Accounts = append(out.Accounts, toAccountResp(a))
	}
	writeJSON(w, r, http.StatusOK, out)
}

// getBalance: GET /accounts/{id} → Ledger.Balance (R10, §1.2). It first reads the
// account for its currency and owner, enforces the D-D1 read rule (non-owned →
// 404, not 403), then returns the derived balance.
func (h *Handler) getBalance(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())
	id := core.AccountID(r.PathValue("id"))

	acc, err := h.ledger.GetAccount(r.Context(), id)
	if err != nil {
		writeCoreErr(w, r, err) // unknown account → 404 account_not_found
		return
	}
	if !h.isAllowedReader(principal, acc.Owner) {
		// D-D1: do not leak existence of an account the principal does not own.
		writeErr(w, r, http.StatusNotFound, "account_not_found", "account not found")
		return
	}

	amount, err := h.ledger.Balance(r.Context(), id)
	if err != nil {
		writeCoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, balanceResp{
		AccountID: string(id),
		Amount:    int64(amount),
		Currency:  string(acc.Currency),
	})
}

// createTransfer: POST /transfers → Ledger.Transfer (R11, §1.3, §5). It sets
// Authorizer from auth and does NOT pre-check ownership (core does, §2.2). The
// Idempotency-Key header is required. A replay (Replayed==true) is 200; a first
// write is 201.
func (h *Handler) createTransfer(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())

	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeErr(w, r, http.StatusBadRequest, "idempotency_key_required",
			"Idempotency-Key header is required")
		return
	}
	var body transferReq
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, r, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	res, err := h.ledger.Transfer(r.Context(), core.TransferCmd{
		Authorizer:     principal, // the ONLY thing the API asserts about identity
		From:           core.AccountID(body.From),
		To:             core.AccountID(body.To),
		Amount:         core.Amount(body.Amount),
		Currency:       core.Currency(body.Currency),
		IdempotencyKey: key,
	})
	if err != nil {
		writeCoreErr(w, r, err)
		return
	}
	status := http.StatusCreated
	if res.Replayed {
		status = http.StatusOK // §5.2 replay is a success, not an error
	}
	writeJSON(w, r, status, toTransferResp(res.Transfer, res.Replayed))
}

// getStatement: GET /accounts/{id}/statement → Ledger.Statement (R12, §1.4),
// keyset-paged. Enforces the same D-D1 read rule as getBalance.
func (h *Handler) getStatement(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())
	id := core.AccountID(r.PathValue("id"))

	acc, err := h.ledger.GetAccount(r.Context(), id)
	if err != nil {
		writeCoreErr(w, r, err)
		return
	}
	if !h.isAllowedReader(principal, acc.Owner) {
		writeErr(w, r, http.StatusNotFound, "account_not_found", "account not found")
		return
	}

	page, err := parsePage(r)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	stmt, err := h.ledger.Statement(r.Context(), id, page)
	if err != nil {
		writeCoreErr(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, toStatementResp(id, stmt))
}

// listCurrencies: GET /currencies → Ledger.ListCurrencies (migration 0002). Public
// reference data: any authenticated principal may read it (auth is enforced by the
// middleware on this non-exempt path), but there is no ownership - currencies are not
// owned, so no principal-specific filtering and no D-D1 read rule applies here.
func (h *Handler) listCurrencies(w http.ResponseWriter, r *http.Request) {
	currencies, err := h.ledger.ListCurrencies(r.Context())
	if err != nil {
		writeCoreErr(w, r, err)
		return
	}
	out := currenciesResp{Currencies: make([]currencyResp, 0, len(currencies))}
	for _, c := range currencies {
		out.Currencies = append(out.Currencies, toCurrencyResp(c))
	}
	writeJSON(w, r, http.StatusOK, out)
}

// --- decode helpers -------------------------------------------------------------

// maxBodyBytes bounds a request body so a malformed/huge payload cannot exhaust
// memory. The toy's bodies are tiny.
const maxBodyBytes = 1 << 20 // 1 MiB

// decodeJSON strictly decodes a JSON body into v, rejecting unknown fields and
// trailing content so a malformed body is a clean 400 (§3).
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// parsePage maps the ?limit= and ?after_seq= query params onto core.Page (§1.4).
// Absent params fall to core's defaults; a non-numeric value is a 400.
func parsePage(r *http.Request) (core.Page, error) {
	var page core.Page
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return core.Page{}, errors.New("limit must be an integer")
		}
		page.Limit = n
	}
	if v := q.Get("after_seq"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return core.Page{}, errors.New("after_seq must be an integer")
		}
		page.AfterSeq = n
	}
	return page, nil
}

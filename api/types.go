package api

import (
	"time"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// This file holds the JSON request/response shapes - the wire contract Nia's web
// layer codes to (10-api.md §1). Money is int64 minor units on the wire, never a
// float; currency is an opaque label echoed through; timestamps are RFC3339 UTC.

// createAccountReq is the POST /accounts body. owner is NOT accepted from the
// body: the owner is the authenticated principal (§1.1).
type createAccountReq struct {
	Currency      string `json:"currency"`
	AllowNegative bool   `json:"allow_negative"`
}

// accountResp is the POST /accounts response and an element of GET /accounts.
type accountResp struct {
	ID            string `json:"id"`
	Owner         string `json:"owner"`
	Currency      string `json:"currency"`
	AllowNegative bool   `json:"allow_negative"`
	CreatedAt     string `json:"created_at"`
}

// accountsResp is the GET /accounts list body (D-C1).
type accountsResp struct {
	Accounts []accountResp `json:"accounts"`
}

// balanceResp is the GET /accounts/{id} body. amount is minor units (§1.2).
type balanceResp struct {
	AccountID string `json:"account_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
}

// transferReq is the POST /transfers body. The idempotency key is a header, not a
// body field (§5).
type transferReq struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// postingResp is one leg in a transfer or statement response.
type postingResp struct {
	ID         string `json:"id,omitempty"`
	TransferID string `json:"transfer_id,omitempty"`
	AccountID  string `json:"account_id"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency,omitempty"`
	Seq        int64  `json:"seq"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// transferResp is the POST /transfers response (§1.3). replayed selects 200 vs 201.
type transferResp struct {
	TransferID string        `json:"transfer_id"`
	From       string        `json:"from"`
	To         string        `json:"to"`
	Amount     int64         `json:"amount"`
	Currency   string        `json:"currency"`
	Replayed   bool          `json:"replayed"`
	CreatedAt  string        `json:"created_at"`
	Postings   []postingResp `json:"postings"`
}

// statementResp is the GET /accounts/{id}/statement body (§1.4), keyset-paged.
type statementResp struct {
	AccountID    string        `json:"account_id"`
	Postings     []postingResp `json:"postings"`
	NextAfterSeq int64         `json:"next_after_seq"`
	HasMore      bool          `json:"has_more"`
}

// reversalResp is the POST /entries/{id}/reverse body: the compensating transfer,
// plus the id of the entry it compensates. A reversal IS a transfer (R5: postings
// are append-only, a correction is an INSERT, never an edit), so the shape MIRRORS
// transferResp rather than nesting it - `reverses_entry_id` is the only honest
// difference. replayed selects 200 vs 201, for the same reason POST /transfers
// carries it: the reverse is sent with an Idempotency-Key, so a retried reverse
// returns the original correction instead of posting a second one (R4).
//
// Flat, and named `reverses_entry_id`, because this is the contract Nia's web layer
// already codes to (`ReversalBody` in web/src/api/types.ts, written as a declared
// stub while this endpoint was in flight). The consumer published first and the
// shape she derived mirrors the house style; api is the owner of the wire contract
// and adopts it rather than making the consumer rewrite to a nested equivalent.
type reversalResp struct {
	TransferID      string        `json:"transfer_id"`
	ReversesEntryID string        `json:"reverses_entry_id"`
	From            string        `json:"from"`
	To              string        `json:"to"`
	Amount          int64         `json:"amount"`
	Currency        string        `json:"currency"`
	Replayed        bool          `json:"replayed"`
	CreatedAt       string        `json:"created_at"`
	Postings        []postingResp `json:"postings"`
}

// toReversalResp renders the compensating transfer plus the link back to the entry
// it reverses. It reuses toTransferResp so the two bodies cannot drift.
func toReversalResp(entryID string, t core.Transfer, replayed bool) reversalResp {
	c := toTransferResp(t, replayed)
	return reversalResp{
		TransferID:      c.TransferID,
		ReversesEntryID: entryID,
		From:            c.From,
		To:              c.To,
		Amount:          c.Amount,
		Currency:        c.Currency,
		Replayed:        c.Replayed,
		CreatedAt:       c.CreatedAt,
		Postings:        c.Postings,
	}
}

// currencyResp is one element of GET /currencies - the public reference metadata
// from migration 0002. minor_unit is the decimal scale; symbol is the display glyph.
type currencyResp struct {
	Code      string `json:"code"`
	MinorUnit int    `json:"minor_unit"`
	Symbol    string `json:"symbol"`
}

// currenciesResp is the GET /currencies list body.
type currenciesResp struct {
	Currencies []currencyResp `json:"currencies"`
}

func toCurrencyResp(c core.CurrencyMeta) currencyResp {
	return currencyResp{Code: c.Code, MinorUnit: c.MinorUnit, Symbol: c.Symbol}
}

// holdReq is the POST /accounts/{id}/holds body. The account id is the path param,
// not a body field; the idempotency key is a header (like transfers).
type holdReq struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// holdResp is the POST /accounts/{id}/holds response.
type holdResp struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

func toHoldResp(h core.Hold) holdResp {
	return holdResp{
		ID:        string(h.ID),
		AccountID: string(h.Account),
		Amount:    int64(h.Amount),
		Currency:  string(h.Currency),
		Active:    h.Active,
		CreatedAt: h.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toAccountResp(a core.Account) accountResp {
	return accountResp{
		ID:            string(a.ID),
		Owner:         string(a.Owner),
		Currency:      string(a.Currency),
		AllowNegative: a.AllowNegative,
		CreatedAt:     a.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toTransferResp(t core.Transfer, replayed bool) transferResp {
	postings := make([]postingResp, 0, len(t.Postings))
	for _, p := range t.Postings {
		postings = append(postings, postingResp{
			AccountID: string(p.Account),
			Amount:    int64(p.Amount),
			Seq:       p.Seq,
		})
	}
	return transferResp{
		TransferID: t.ID,
		From:       string(t.From),
		To:         string(t.To),
		Amount:     int64(t.Amount),
		Currency:   string(t.Currency),
		Replayed:   replayed,
		CreatedAt:  t.CreatedAt.UTC().Format(time.RFC3339),
		Postings:   postings,
	}
}

func toStatementResp(id core.AccountID, page core.StatementPage) statementResp {
	postings := make([]postingResp, 0, len(page.Postings))
	for _, p := range page.Postings {
		postings = append(postings, postingResp{
			ID:         p.ID,
			TransferID: p.TransferID,
			AccountID:  string(p.Account),
			Amount:     int64(p.Amount),
			Currency:   string(p.Currency),
			Seq:        p.Seq,
			CreatedAt:  p.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return statementResp{
		AccountID:    string(id),
		Postings:     postings,
		NextAfterSeq: page.NextAfterSeq,
		HasMore:      page.HasMore,
	}
}

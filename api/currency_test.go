package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// GET /currencies requires an authenticated principal: no bearer token is a 401,
// never a leak of the reference data (§2 auth boundary applies to every non-exempt
// path, and /currencies is not exempt).
func TestListCurrencies_MissingToken_401(t *testing.T) {
	h, _ := realHandler()
	rec := do(t, h, http.MethodGet, "/currencies", "", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d (%s)", rec.Code, rec.Body.String())
	}
	if code := decodeErr(t, rec).Code; code != "unauthenticated" {
		t.Fatalf("want code unauthenticated, got %q", code)
	}
}

// Any valid bearer token may read the currencies: there is no ownership on public
// reference data, so an ordinary principal (not an admin, owning nothing) gets 200
// and the seeded rows, ordered by code.
func TestListCurrencies_Authenticated_ReturnsSeeded(t *testing.T) {
	h, ms := realHandler()
	ms.currencies = []core.CurrencyMeta{
		{Code: "USD", MinorUnit: 2, Symbol: "$"},
		{Code: "EUR", MinorUnit: 2, Symbol: "€"},
		{Code: "JPY", MinorUnit: 0, Symbol: "¥"},
	}

	rec := do(t, h, http.MethodGet, "/currencies", "alice", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var body currenciesResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	want := []currencyResp{
		{Code: "EUR", MinorUnit: 2, Symbol: "€"},
		{Code: "JPY", MinorUnit: 0, Symbol: "¥"},
		{Code: "USD", MinorUnit: 2, Symbol: "$"},
	}
	if len(body.Currencies) != len(want) {
		t.Fatalf("want %d currencies, got %d: %+v", len(want), len(body.Currencies), body.Currencies)
	}
	for i := range want {
		if body.Currencies[i] != want[i] {
			t.Fatalf("row %d: want %+v, got %+v", i, want[i], body.Currencies[i])
		}
	}
}

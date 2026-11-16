package core

import (
	"context"
	"testing"
)

// ListCurrencies is a thin read-through to the Store: the engine does no filtering
// or ordering of its own, so this proves the engine surfaces exactly what the Store
// returns (the ordering guarantee is the adapter's, exercised in internal/sqlite).
func TestListCurrencies_ReturnsSeededRows(t *testing.T) {
	store := newFakeStore()
	store.currencies = []CurrencyMeta{
		{Code: "EUR", MinorUnit: 2, Symbol: "€"},
		{Code: "GBP", MinorUnit: 2, Symbol: "£"},
		{Code: "JPY", MinorUnit: 0, Symbol: "¥"},
		{Code: "USD", MinorUnit: 2, Symbol: "$"},
	}
	l := New(store)

	got, err := l.ListCurrencies(context.Background())
	if err != nil {
		t.Fatalf("list currencies: %v", err)
	}
	want := []CurrencyMeta{
		{Code: "EUR", MinorUnit: 2, Symbol: "€"},
		{Code: "GBP", MinorUnit: 2, Symbol: "£"},
		{Code: "JPY", MinorUnit: 0, Symbol: "¥"},
		{Code: "USD", MinorUnit: 2, Symbol: "$"},
	}
	if len(got) != len(want) {
		t.Fatalf("want %d currencies, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d: want %+v, got %+v", i, want[i], got[i])
		}
	}
}

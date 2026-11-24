package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Stephen-Kamau/ledger-wallet/core"
	"github.com/Stephen-Kamau/ledger-wallet/internal/sqlite"
	"github.com/Stephen-Kamau/ledger-wallet/migrations"
)

// migratedStore opens a fresh WAL DB, applies the REAL migrator (0001 + 0002, which
// creates and seeds the currencies table), and wraps it in the adapter. This proves
// ListCurrencies reads the actual seeded reference data, not a fake - the currencies
// table does not exist in the adapter's embeddedSchema, so this can only pass against
// a genuinely migrated database.
func migratedStore(t *testing.T) *sqlite.Store {
	t.Helper()
	const dsn = "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_txlock=immediate"
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "currencies.db")+dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return sqlite.New(db)
}

// ListCurrencies returns every seeded row, ordered by code, against a real migrated DB.
func TestListCurrencies_MigratedDB(t *testing.T) {
	store := migratedStore(t)

	got, err := store.ListCurrencies(context.Background())
	if err != nil {
		t.Fatalf("list currencies: %v", err)
	}
	// 0002 seeds four rows; ORDER BY code gives EUR, GBP, JPY, USD.
	want := []core.CurrencyMeta{
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

package tests

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Stephen-Kamau/ledger-wallet/core"
	"github.com/Stephen-Kamau/ledger-wallet/internal/sqlite"
	"github.com/Stephen-Kamau/ledger-wallet/migrations"
)

// TestR14_ReRunIsNoOp proves migrations are idempotent: applying twice records each
// version exactly once and the second run changes nothing (R14).
func TestR14_ReRunIsNoOp(t *testing.T) {
	db := openMigrated(t, filepath.Join(t.TempDir(), "r14.db"), true)

	names, err := migrations.Names()
	if err != nil {
		t.Fatalf("R14: Names: %v", err)
	}
	want := len(names)

	before := versionsOf(t, db)
	if len(before) != want {
		t.Fatalf("R14: want %d recorded versions after first apply, got %v", want, before)
	}
	// Re-run: must be a clean no-op.
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("R14: second Apply errored: %v", err)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM schema_migrations`); n != int64(want) {
		t.Fatalf("R14: re-run changed the ledger: %d rows, want %d", n, want)
	}
	// A third run for good measure.
	if err := migrations.Apply(db); err != nil {
		t.Fatalf("R14: third Apply errored: %v", err)
	}
	if got := versionsOf(t, db); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatalf("R14: versions drifted on re-run: %v != %v", got, before)
	}
}

// TestR15_SchemaEvolved confirms the post-v1 evolution actually landed: the currencies
// table exists with its seed, and it was applied as a distinct numbered migration (R15).
func TestR15_SchemaEvolved(t *testing.T) {
	db := openMigrated(t, filepath.Join(t.TempDir(), "r15.db"), true)
	if n := scalar(t, db, `SELECT COUNT(*) FROM currencies`); n != 4 {
		t.Fatalf("R15: want 4 seeded currencies, got %d", n)
	}
	if n := scalar(t, db, `SELECT minor_unit FROM currencies WHERE code='JPY'`); n != 0 {
		t.Fatalf("R15: JPY minor_unit want 0, got %d", n)
	}
	if n := scalar(t, db, `SELECT minor_unit FROM currencies WHERE code='USD'`); n != 2 {
		t.Fatalf("R15: USD minor_unit want 2, got %d", n)
	}
	versions := versionsOf(t, db)
	if len(versions) < 2 || versions[1] != "0002" {
		t.Fatalf("R15: evolution not recorded as 0002: %v", versions)
	}
}

// TestR16_BootstrapParity proves a fresh DB built by the migrator is schema-identical and
// seed-identical to a DB evolved step by step (0001, used, then the rest applied later).
// Parity is a claim about schema and seed, not transactional data -- the evolved DB's
// accounts and postings are expected to differ and are excluded from the comparison
// (30-schema §3.3).
//
// The evolved path walks migrations.Names() rather than a written-out list of filenames.
// A hardcoded list is a copy of the subject that goes stale the moment a migration is
// added: the new file would simply never be applied to the evolved DB, and the parity it
// claims to prove would quietly stop covering it.
func TestR16_BootstrapParity(t *testing.T) {
	// Fresh: empty DB -> all migrations at once.
	fresh := openMigrated(t, filepath.Join(t.TempDir(), "fresh.db"), true)

	names, err := migrations.Names()
	if err != nil {
		t.Fatalf("R16: Names: %v", err)
	}
	if len(names) < 2 {
		t.Fatalf("R16: parity needs at least two migrations to be a step-by-step claim, got %v", names)
	}

	// Evolved: empty DB -> 0001 only, USED, then every later migration one at a time.
	evolved := openMigrated(t, filepath.Join(t.TempDir(), "evolved.db"), false)
	for i, name := range names {
		if err := migrations.ApplyOne(evolved, name); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if i == 0 {
			// Exercise the v1 schema before evolving it: accounts, transfers, postings.
			// Real data in the DB is what makes the later migrations a genuine evolution
			// rather than a second fresh build.
			runSomeTransfers(t, evolved)
		}
	}

	if a, b := schemaOf(t, fresh), schemaOf(t, evolved); a != b {
		t.Fatalf("R16 schema parity failed:\n--- fresh ---\n%s\n--- evolved ---\n%s", a, b)
	}
	if a, b := currencySeedOf(t, fresh), currencySeedOf(t, evolved); fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatalf("R16 seed parity failed: %v != %v", a, b)
	}
	if a, b := versionsOf(t, fresh), versionsOf(t, evolved); fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatalf("R16 version parity failed: %v != %v", a, b)
	}
}

// TestR5_AppendOnlyViaMigratedSchema proves the append-only triggers carried by 0001
// (not the adapter's embedded schema) refuse a direct UPDATE/DELETE on postings, while a
// compensating correction -- which is an INSERT -- still succeeds (R5).
func TestR5_AppendOnlyViaMigratedSchema(t *testing.T) {
	l, db := newLedger(t)
	ctx := context.Background()
	sys := newSystemAccount(t, l)
	a := newAccount(t, l, "alice")
	b := newAccount(t, l, "bob")
	fund(t, l, sys, a, 50)
	if _, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "alice", From: a, To: b, Amount: 5, Currency: usd, IdempotencyKey: "t",
	}); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE postings SET amount = amount + 1`); err == nil {
		t.Fatal("R5: UPDATE on postings should be refused by the trigger")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM postings`); err == nil {
		t.Fatal("R5: DELETE on postings should be refused by the trigger")
	}

	// A compensating correction is a NEW transfer (INSERT), and must still work.
	if _, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "bob", From: b, To: a, Amount: 5, Currency: usd, IdempotencyKey: "reverse",
	}); err != nil {
		t.Fatalf("R5: compensating transfer should succeed, got %v", err)
	}
	if got := balance(t, l, b); got != 0 {
		t.Fatalf("R5: after reverse, b should be 0, got %d", got)
	}
}

// --- parity helpers --------------------------------------------------------------

// runSomeTransfers wires the engine over an already-migrated (0001-only) DB and posts a
// mint + a transfer, so the evolved DB carries real user data before 0002 is applied.
func runSomeTransfers(t *testing.T, db *sql.DB) {
	t.Helper()
	l := core.New(sqlite.New(db))
	sys := newSystemAccount(t, l)
	a := newAccount(t, l, "alice")
	fund(t, l, sys, a, 25)
}

// schemaOf returns the canonical schema DDL (tables, indexes, triggers) excluding
// SQLite's internal objects, ordered deterministically.
func schemaOf(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT type, name, COALESCE(sql,'') FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		t.Fatalf("schemaOf: %v", err)
	}
	defer rows.Close()
	var out string
	for rows.Next() {
		var typ, name, ddl string
		if err := rows.Scan(&typ, &name, &ddl); err != nil {
			t.Fatalf("schemaOf scan: %v", err)
		}
		out += fmt.Sprintf("%s|%s|%s\n", typ, name, ddl)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("schemaOf rows: %v", err)
	}
	return out
}

func currencySeedOf(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT code, minor_unit, symbol, created_at FROM currencies ORDER BY code`)
	if err != nil {
		t.Fatalf("currencySeedOf: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code, symbol, createdAt string
		var minor int64
		if err := rows.Scan(&code, &minor, &symbol, &createdAt); err != nil {
			t.Fatalf("currencySeedOf scan: %v", err)
		}
		out = append(out, fmt.Sprintf("%s|%d|%s|%s", code, minor, symbol, createdAt))
	}
	return out
}

func versionsOf(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("versionsOf: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("versionsOf scan: %v", err)
		}
		out = append(out, v)
	}
	return out
}

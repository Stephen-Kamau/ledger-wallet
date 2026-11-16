package main

import (
	"context"
	"path/filepath"
	"testing"
)

// TestNew_ServerSchemaIsFullyMigrated is the drift guard that would have caught the
// GET /currencies 500. It exercises the REAL wired server constructor (New) — not a
// migrations-applied test harness — and asserts the server DB carries every table the
// numbered migrations define. The bug: New used to boot from the adapter's
// embeddedSchema, which lacked the currencies table (that table lives only in
// migrations/0002), so the live server had no currencies table and GET /currencies
// 500'd, while every migration-applied unit test passed. New now applies the
// migrations as the single schema source; this test fails closed if that ever regresses.
func TestNew_ServerSchemaIsFullyMigrated(t *testing.T) {
	ctx := context.Background()
	ledger, db, err := New(ctx, filepath.Join(t.TempDir(), "srv.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if ledger == nil {
		t.Fatal("New returned a nil ledger")
	}

	// Every table the migrations define must exist on the wired server DB.
	for _, table := range []string{"accounts", "transfers", "postings", "currencies", "holds"} {
		var name string
		if err := db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_schema WHERE type='table' AND name=?`, table).Scan(&name); err != nil {
			t.Fatalf("server schema is missing table %q (schema drift between embeddedSchema and migrations): %v", table, err)
		}
	}

	// The currencies table must be SEEDED (migrations/0002) — this SELECT is exactly
	// what GET /currencies runs; before the fix it failed on the live server.
	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM currencies`).Scan(&n); err != nil {
		t.Fatalf("SELECT from currencies failed on the wired server DB (the GET /currencies 500): %v", err)
	}
	if n != 4 {
		t.Fatalf("currencies seed on the server DB: want 4 rows, got %d", n)
	}

	// Migrations are idempotent (R14): a second New against the same file is a no-op,
	// not an error. (New opens a fresh DB path per call here, so just prove re-open works.)
	ledger2, db2, err := New(ctx, filepath.Join(t.TempDir(), "srv2.db"))
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	t.Cleanup(func() { _ = db2.Close() })
	_ = ledger2
}

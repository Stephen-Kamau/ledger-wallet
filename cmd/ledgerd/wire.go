// Package main (cmd/ledgerd) is the server entrypoint. It opens the SQLite DB,
// applies the numbered migrations as the database's single schema source, and
// constructs the core.Ledger ready for the api layer (Dakari) to mount. It builds no
// HTTP itself: New is the single wiring seam api calls.
package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Stephen-Kamau/ledger-wallet/core"
	"github.com/Stephen-Kamau/ledger-wallet/internal/sqlite"
	"github.com/Stephen-Kamau/ledger-wallet/migrations"
)

// New opens the SQLite database at dbPath with the §5 pragmas, brings it to the full
// migrated schema, and returns the wired core.Ledger plus the underlying *sql.DB (so
// the caller can close it and so tests/ can assert invariants against the DB directly).
// This is the constructor the api layer and cmd use; it is the only place the concrete
// adapter is named.
//
// The schema comes from the numbered migrations (migrations.Apply) as the SINGLE
// source — NOT the adapter's embeddedSchema. embeddedSchema is for the adapter's
// standalone tests only; when it drifted from the migrations (it lacked the currencies
// table, which lives in migrations/0002), a server booted from it had no currencies
// table and GET /currencies 500'd. Applying migrations here means the running server
// always has everything the migrations define — currencies, holds, and every future
// one — so that class of drift cannot recur. Migrations are idempotent (R14), so New
// is safe to call against a fresh or an already-migrated database.
func New(ctx context.Context, dbPath string) (core.Ledger, *sql.DB, error) {
	store, err := sqlite.OpenRaw(ctx, dbPath)
	if err != nil {
		return nil, nil, err
	}
	if err := migrations.Apply(store.DB()); err != nil {
		_ = store.DB().Close()
		return nil, nil, fmt.Errorf("ledgerd: apply migrations: %w", err)
	}
	return core.New(store), store.DB(), nil
}

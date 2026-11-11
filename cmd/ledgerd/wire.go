// Package main (cmd/ledgerd) is the server entrypoint. It opens the SQLite DB,
// applies the embedded schema, and constructs the core.Ledger ready for the api
// layer (Dakari) to mount. It builds no HTTP itself: New is the single wiring seam
// api calls.
package main

import (
	"context"
	"database/sql"

	"github.com/Stephen-Kamau/ledger-wallet/core"
	"github.com/Stephen-Kamau/ledger-wallet/internal/sqlite"
)

// New opens the SQLite database at dbPath with the §5 pragmas, ensures the schema,
// and returns the wired core.Ledger plus the underlying *sql.DB (so the caller can
// close it and so tests/ can assert invariants against the DB directly). This is the
// constructor the api layer and cmd use; it is the only place the concrete adapter
// is named.
func New(ctx context.Context, dbPath string) (core.Ledger, *sql.DB, error) {
	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return nil, nil, err
	}
	return core.New(store), store.DB(), nil
}

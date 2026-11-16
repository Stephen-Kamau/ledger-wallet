// Package sqlite implements core.Store over SQLite using the pure-Go
// modernc.org/sqlite driver through database/sql. It realises the concurrency
// model of 00-core.md §5: WAL journaling, BEGIN IMMEDIATE on every write
// transaction (so the write lock is taken at transaction start, not at first
// write), a busy_timeout so a blocked writer waits rather than errors, and
// foreign_keys ON. The R3 sufficiency check and seq assignment (D-B1) run inside
// the write transaction.
//
// This adapter is unreachable outside the module by design (internal/): nobody
// codes to the adapter, everybody codes to the core.Store port.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// embeddedSchema is a minimal, idempotent schema so this adapter and its tests run
// standalone. Kofi's migrations/ (0001_init.sql) are the authoritative schema and
// supersede/align with this; the DDL here mirrors 00-core.md §6 and 30-schema §1.4,
// including the UNIQUE(account_id, seq) strengthening (D-B1) and the append-only
// triggers (R5). journal_mode is set per-connection via the DSN, not here.
const embeddedSchema = `
CREATE TABLE IF NOT EXISTS accounts (
  id             TEXT    PRIMARY KEY,
  owner          TEXT    NOT NULL,
  currency       TEXT    NOT NULL,
  allow_negative INTEGER NOT NULL DEFAULT 0 CHECK (allow_negative IN (0,1)),
  created_at     TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_accounts_owner ON accounts(owner);

CREATE TABLE IF NOT EXISTS transfers (
  id              TEXT    PRIMARY KEY,
  owner           TEXT    NOT NULL,
  idempotency_key TEXT    NOT NULL,
  from_account    TEXT    NOT NULL REFERENCES accounts(id),
  to_account      TEXT    NOT NULL REFERENCES accounts(id),
  amount          INTEGER NOT NULL CHECK (amount > 0),
  currency        TEXT    NOT NULL,
  created_at      TEXT    NOT NULL,
  UNIQUE (owner, idempotency_key)
);

CREATE TABLE IF NOT EXISTS postings (
  id          TEXT    PRIMARY KEY,
  transfer_id TEXT    NOT NULL REFERENCES transfers(id),
  account_id  TEXT    NOT NULL REFERENCES accounts(id),
  amount      INTEGER NOT NULL,
  currency    TEXT    NOT NULL,
  seq         INTEGER NOT NULL,
  created_at  TEXT    NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_postings_account_seq ON postings(account_id, seq);

CREATE TRIGGER IF NOT EXISTS postings_no_update
BEFORE UPDATE ON postings
BEGIN
  SELECT RAISE(ABORT, 'postings are append-only (R5): UPDATE forbidden');
END;

CREATE TRIGGER IF NOT EXISTS postings_no_delete
BEFORE DELETE ON postings
BEGIN
  SELECT RAISE(ABORT, 'postings are append-only (R5): DELETE forbidden');
END;

CREATE TABLE IF NOT EXISTS holds (
  id              TEXT    PRIMARY KEY,
  account_id      TEXT    NOT NULL REFERENCES accounts(id),
  owner           TEXT    NOT NULL,
  idempotency_key TEXT    NOT NULL,
  amount          INTEGER NOT NULL CHECK (amount > 0),
  currency        TEXT    NOT NULL,
  active          INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0,1)),
  created_at      TEXT    NOT NULL,
  released_at     TEXT,
  UNIQUE (owner, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_holds_account_active ON holds(account_id, active);
`

// Store adapts a *sql.DB to the core.Store port.
type Store struct {
	db *sql.DB
}

// New wraps an already-open *sql.DB. The DB must have been opened with the pragmas
// §5 requires; use Open for the canonical wiring.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// dsnPragmas carries the per-connection and persistent PRAGMAs §5 requires:
//   - journal_mode=WAL  : readers never block the single writer (persistent header)
//   - busy_timeout=5000 : a blocked BEGIN IMMEDIATE waits, not errors (per-conn)
//   - foreign_keys=ON   : REFERENCES are enforced (per-conn, resets each Open)
//
// _txlock=immediate makes database/sql's BeginTx issue BEGIN IMMEDIATE so the write
// lock is acquired at transaction start - the heart of the double-spend defence (§5).
const dsnPragmas = "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_txlock=immediate"

// maxOpenConns caps the database/sql pool (see Open). 16 is where Kofi's read
// fan-out throughput recovered; it comfortably clears the single writer plus
// concurrent WAL readers without letting the pool churn fresh connections.
const maxOpenConns = 16

// OpenRaw opens (or creates) the SQLite database at path with the §5 pragmas and the
// capped connection pool, but applies NO schema. The SERVER path uses this and then
// applies the numbered migrations as its SINGLE schema source (cmd/ledgerd.New), so
// the running server never boots from the adapter's embeddedSchema. embeddedSchema
// (applied by Open) exists only to let the adapter's own tests stand alone, and must
// never again be the thing the live server depends on: when it drifted from the
// migrations (the currencies table lived only in migrations/0002), the server booted
// without that table and GET /currencies 500'd while every migration-applied unit test
// stayed green. Migrations as the server's one source is what forecloses that drift.
func OpenRaw(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	// Cap the pool. database/sql opens a fresh connection per concurrent caller by
	// default, and every fresh SQLite connection re-runs the DSN PRAGMAs; under read
	// fan-out that churn collapses read throughput (measured ~193 rps at concurrency
	// 64, recovered when capped). Keep idle == max so warm connections are reused
	// rather than reopened. WAL permits one writer regardless; the cap just stops the
	// readers from thrashing.
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)
	db.SetConnMaxLifetime(0) // never expire a warm connection
	return &Store{db: db}, nil
}

// Open opens (or creates) the SQLite database at path with the §5 pragmas AND applies
// the embedded standalone schema, returning a ready Store. Used by the adapter's own
// tests so they need no migrations dependency; the server uses OpenRaw + migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	store, err := OpenRaw(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := EnsureSchema(ctx, store.db); err != nil {
		_ = store.db.Close()
		return nil, err
	}
	return store, nil
}

// EnsureSchema applies the embedded schema idempotently. Kofi's migrations layer
// supersedes this in the wired app; it exists so core's adapter tests stand alone.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, embeddedSchema); err != nil {
		return fmt.Errorf("sqlite schema: %w", err)
	}
	return nil
}

// DB exposes the underlying handle for the wiring helper and concurrency tests that
// assert invariants directly against the database (30-schema §4.5).
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the underlying DB.
func (s *Store) Close() error { return s.db.Close() }

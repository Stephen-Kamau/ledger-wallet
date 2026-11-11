// Package tests holds the cross-stream suite: it drives the real engine (core) over
// the real SQLite adapter (internal/sqlite) on a real WAL database built by the
// migrator (migrations). Amara's fake-Store unit tests prove the engine's logic; this
// suite proves the storage adapter and the §5 concurrency model hold under real
// contention (30-schema §4). Deliberately a different level than the fake-Store tests.
package tests

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Stephen-Kamau/ledger-wallet/core"
	"github.com/Stephen-Kamau/ledger-wallet/internal/sqlite"
	"github.com/Stephen-Kamau/ledger-wallet/migrations"
)

// dsn carries the §5 pragmas on every connection the pool opens:
//   - journal_mode=WAL   : readers never block the single writer (persistent header)
//   - busy_timeout=30000 : a blocked BEGIN IMMEDIATE waits, not errors, even under the
//     heavy fan-out of the concurrency tests (30-schema §4.5)
//   - foreign_keys=ON    : REFERENCES are enforced (per-connection, resets each Open)
//   - _txlock=immediate  : database/sql's BeginTx issues BEGIN IMMEDIATE, taking the
//     write lock at transaction start -- the heart of the double-spend defence (§5)
const dsn = "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)&_pragma=foreign_keys(ON)&_txlock=immediate"

const (
	sysOwner = core.PrincipalID("system")
	usd      = core.Currency("USD")
)

// newLedger spins a fresh WAL DB in a temp dir, applies ALL migrations via the real
// migrator, and wires the real sqlite.Store into core.New. It returns the raw *sql.DB
// so tests can assert invariants directly against the database (30-schema §4.5).
func newLedger(t *testing.T) (core.Ledger, *sql.DB) {
	t.Helper()
	db := openMigrated(t, filepath.Join(t.TempDir(), "ledger.db"), true)
	return core.New(sqlite.New(db)), db
}

// openMigrated opens a DB at path with the §5 DSN. When applyAll is true it runs the
// whole migrator; otherwise it leaves the DB empty for step-by-step parity tests.
func openMigrated(t *testing.T, path string, applyAll bool) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path+dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Cap the pool. WAL serializes writers on the lock and lets readers proceed, but an
	// UNBOUNDED database/sql pool churns fresh connections under high read fan-out -- each
	// new modernc connection re-runs the DSN PRAGMAs, which dominates and collapses read
	// throughput. A bounded, reused set reflects a real deployment (the adapter's own Open
	// documents a capped pool for the single-writer model).
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	t.Cleanup(func() { _ = db.Close() })
	if applyAll {
		if err := migrations.Apply(db); err != nil {
			t.Fatalf("apply migrations: %v", err)
		}
	}
	return db
}

// newAccount creates a normal (floored-at-zero) account for an owner.
func newAccount(t *testing.T, l core.Ledger, owner core.PrincipalID) core.AccountID {
	t.Helper()
	return mkAccount(t, l, owner, false)
}

// newSystemAccount creates the single AllowNegative mint account, owned by sysOwner.
func newSystemAccount(t *testing.T, l core.Ledger) core.AccountID {
	t.Helper()
	return mkAccount(t, l, sysOwner, true)
}

func mkAccount(t *testing.T, l core.Ledger, owner core.PrincipalID, allowNeg bool) core.AccountID {
	t.Helper()
	a, err := l.CreateAccount(context.Background(), core.CreateAccountCmd{
		Owner: owner, Currency: usd, AllowNegative: allowNeg,
	})
	if err != nil {
		t.Fatalf("create account (owner=%s): %v", owner, err)
	}
	return a.ID
}

// fund mints amount into acct from the system mint account (R2: a mint is a transfer
// from the AllowNegative system account, authorised by the system owner).
func fund(t *testing.T, l core.Ledger, sys, acct core.AccountID, amount core.Amount) {
	t.Helper()
	_, err := l.Transfer(context.Background(), core.TransferCmd{
		Authorizer: sysOwner, From: sys, To: acct,
		Amount: amount, Currency: usd,
		IdempotencyKey: fmt.Sprintf("mint-%s-%d", acct, amount),
	})
	if err != nil {
		t.Fatalf("fund %s with %d: %v", acct, amount, err)
	}
}

// balance reads an account's derived balance via the engine.
func balance(t *testing.T, l core.Ledger, id core.AccountID) core.Amount {
	t.Helper()
	b, err := l.Balance(context.Background(), id)
	if err != nil {
		t.Fatalf("balance %s: %v", id, err)
	}
	return b
}

// isInfraBusy reports whether an error is a retryable SQLITE_BUSY/locked condition
// rather than an invariant outcome. Under busy_timeout this should be vanishingly
// rare, but it is classified separately and retried, never folded into the invariant
// count (30-schema §4.5).
func isInfraBusy(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "busy") || strings.Contains(s, "locked")
}

// transferRetry runs a transfer, retrying only on infra busy/locked errors. It returns
// nil on success, core.ErrInsufficientFunds on an R3 rejection, or any other error
// verbatim. infraRetries counts how many busy retries it absorbed.
func transferRetry(ctx context.Context, l core.Ledger, cmd core.TransferCmd) (err error, infraRetries int) {
	for attempt := 0; attempt < 50; attempt++ {
		_, err = l.Transfer(ctx, cmd)
		if !isInfraBusy(err) {
			return err, infraRetries
		}
		infraRetries++
	}
	return err, infraRetries
}

// barrier releases n goroutines simultaneously to maximise write-lock contention.
func barrier(n int) (ready *sync.WaitGroup, start chan struct{}) {
	ready = &sync.WaitGroup{}
	ready.Add(n)
	start = make(chan struct{})
	return ready, start
}

// scalar runs a single-int64-column query against the raw DB.
func scalar(t *testing.T, db *sql.DB, q string, args ...any) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRowContext(context.Background(), q, args...).Scan(&v); err != nil {
		t.Fatalf("scalar %q: %v", q, err)
	}
	return v
}

// assertInvariants is the shared oracle asserted after every concurrency test. It is
// the "zero failed invariants" surface R19 depends on, factored out so it runs
// everywhere (30-schema §4.5). It returns the number of invariant violations found so
// a caller (R19) can assert it is zero.
func assertInvariants(t *testing.T, db *sql.DB, l core.Ledger, accounts ...core.AccountID) int {
	t.Helper()
	failed := 0

	// R2 conservation: the whole ledger sums to zero (mint from the system account nets zero).
	if total := scalar(t, db, `SELECT COALESCE(SUM(amount),0) FROM postings`); total != 0 {
		t.Errorf("R2 conservation violated: ledger sums to %d, want 0", total)
		failed++
	}

	// R3: no NORMAL account (allow_negative=0) holds a negative derived balance.
	if neg := scalar(t, db, `
		SELECT COUNT(*) FROM accounts a WHERE a.allow_negative = 0
		AND (SELECT COALESCE(SUM(amount),0) FROM postings p WHERE p.account_id = a.id) < 0`); neg != 0 {
		t.Errorf("R3 violated: %d normal account(s) negative", neg)
		failed++
	}

	// Referential: no orphan posting points at a missing transfer.
	if orphans := scalar(t, db, `
		SELECT COUNT(*) FROM postings p WHERE p.transfer_id NOT IN (SELECT id FROM transfers)`); orphans != 0 {
		t.Errorf("referential violated: %d orphan posting(s)", orphans)
		failed++
	}

	// seq integrity: distinct seq count == posting count per account (the UNIQUE index
	// guarantees it; this asserts it held).
	if dup := scalar(t, db, `
		SELECT COUNT(*) FROM (SELECT account_id FROM postings GROUP BY account_id
		HAVING COUNT(DISTINCT seq) <> COUNT(*))`); dup != 0 {
		t.Errorf("seq integrity violated: %d account(s) with a seq collision", dup)
		failed++
	}

	// R10: every named account's engine-derived balance == the SUM of its postings
	// (no stored balance to drift).
	for _, id := range accounts {
		derived := scalar(t, db, `SELECT COALESCE(SUM(amount),0) FROM postings WHERE account_id = ?`, string(id))
		if got := balance(t, l, id); int64(got) != derived {
			t.Errorf("R10 violated: account %s engine balance %d != posting sum %d", id, got, derived)
			failed++
		}
	}
	return failed
}

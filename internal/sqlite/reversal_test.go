package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Stephen-Kamau/ledger-wallet/core"
	"github.com/Stephen-Kamau/ledger-wallet/internal/sqlite"
	"github.com/Stephen-Kamau/ledger-wallet/migrations"
)

// ledgerOn builds a ledger over a real SQLite file whose schema came from the
// named source. The two sources are the two that exist in this repo and they are
// tested SEPARATELY on purpose:
//
//   - "embedded" is the adapter's own standalone schema, used by these tests.
//   - "migrations" is what the SERVER boots from (cmd/ledgerd uses OpenRaw + Apply).
//
// collision-v2 shipped a defect precisely because only the first was exercised:
// the currencies table lived only in migrations/, the adapter's embeddedSchema
// never grew it, and GET /currencies 500'd on the running server while every unit
// test stayed green. Reversal adds no column to either schema, and running the
// whole invariant suite against BOTH is what proves that claim rather than
// asserting it.
func ledgerOn(t *testing.T, source string) (core.Ledger, *sqlite.Store) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")

	switch source {
	case "embedded":
		store, err := sqlite.Open(ctx, path)
		if err != nil {
			t.Fatalf("open (embedded): %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })
		return core.New(store), store
	case "migrations":
		store, err := sqlite.OpenRaw(ctx, path)
		if err != nil {
			t.Fatalf("open (migrations): %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })
		if err := migrations.Apply(store.DB()); err != nil {
			t.Fatalf("apply migrations: %v", err)
		}
		return core.New(store), store
	default:
		t.Fatalf("unknown schema source %q", source)
		return nil, nil
	}
}

// schemaSources are the two schema paths every reversal invariant is proven on.
var schemaSources = []string{"embedded", "migrations"}

// funded mints `fund` into alice and moves `move` to bob, returning the original
// entry. Minting is a transfer from the AllowNegative system account (R2).
func funded(t *testing.T, l core.Ledger, fund, move core.Amount) (core.Account, core.Account, core.Transfer) {
	t.Helper()
	ctx := context.Background()
	sys := mustAcct(t, l, "system", "USD", true)
	a := mustAcct(t, l, "alice", "USD", false)
	b := mustAcct(t, l, "bob", "USD", false)

	if _, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "system", From: sys.ID, To: a.ID,
		Amount: fund, Currency: "USD", IdempotencyKey: "mint",
	}); err != nil {
		t.Fatalf("mint: %v", err)
	}
	res, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "alice", From: a.ID, To: b.ID,
		Amount: move, Currency: "USD", IdempotencyKey: "orig",
	})
	if err != nil {
		t.Fatalf("original transfer: %v", err)
	}
	return a, b, res.Transfer
}

// ledgerTotal is the quantity R2 holds constant: the sum of every posting.
func ledgerTotal(t *testing.T, store *sqlite.Store) int64 {
	t.Helper()
	var sum int64
	if err := store.DB().QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM postings`).Scan(&sum); err != nil {
		t.Fatalf("ledger total: %v", err)
	}
	return sum
}

// R1 + R2 against real SQLite, on both schema sources: the compensating entry nets
// to zero and restores every balance, leaving the ledger-wide sum untouched.
func TestReverse_RealAdapter_BalancedAndConserving(t *testing.T) {
	for _, source := range schemaSources {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			l, store := ledgerOn(t, source)
			a, b, orig := funded(t, l, 100, 30)

			before := ledgerTotal(t, store)

			res, err := l.Reverse(ctx, "bob", orig.ID, "rev-1")
			if err != nil {
				t.Fatalf("reverse: %v", err)
			}

			var sum core.Amount
			for _, p := range res.Transfer.Postings {
				sum += p.Amount
			}
			if sum != 0 {
				t.Fatalf("reversal postings did not net to zero: %d", sum)
			}

			balA, err := l.Balance(ctx, a.ID)
			if err != nil {
				t.Fatalf("balance a: %v", err)
			}
			balB, err := l.Balance(ctx, b.ID)
			if err != nil {
				t.Fatalf("balance b: %v", err)
			}
			if balA != 100 || balB != 0 {
				t.Fatalf("balances not restored: a=%d b=%d", balA, balB)
			}
			if after := ledgerTotal(t, store); after != before {
				t.Fatalf("conservation broken: %d -> %d", before, after)
			}
		})
	}
}

// D-B1 against real SQLite: the adapter assigns each compensating posting a real
// monotonic seq and writes it BACK into the returned result. build-v1 shipped C-1
// (seq-not-in-result) because the fake could not see this; the reversal path gets
// its own guard rather than inheriting the assumption.
func TestReverse_RealAdapter_SeqAssignedAndReturned(t *testing.T) {
	for _, source := range schemaSources {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			l, store := ledgerOn(t, source)
			_, _, orig := funded(t, l, 100, 30)

			res, err := l.Reverse(ctx, "bob", orig.ID, "rev-1")
			if err != nil {
				t.Fatalf("reverse: %v", err)
			}

			for _, p := range res.Transfer.Postings {
				if p.Seq == 0 {
					t.Fatalf("posting %s came back with seq 0; the adapter's seq was not written into the result", p.ID)
				}
				var dbSeq int64
				if err := store.DB().QueryRow(
					`SELECT seq FROM postings WHERE id = ?`, p.ID).Scan(&dbSeq); err != nil {
					t.Fatalf("read seq for %s: %v", p.ID, err)
				}
				if dbSeq != p.Seq {
					t.Fatalf("posting %s: result seq %d != stored seq %d", p.ID, p.Seq, dbSeq)
				}
			}
		})
	}
}

// R5 against real SQLite: the original postings are untouched rows, the reversed
// entry still appears in the statement, and the append-only triggers are still
// armed - an UPDATE or DELETE of a posting is refused by the database itself.
func TestReverse_RealAdapter_AppendOnlyPreserved(t *testing.T) {
	for _, source := range schemaSources {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			l, store := ledgerOn(t, source)
			_, b, orig := funded(t, l, 100, 30)

			if _, err := l.Reverse(ctx, "bob", orig.ID, "rev-1"); err != nil {
				t.Fatalf("reverse: %v", err)
			}

			// Every original posting row survives, with its amount intact.
			for _, p := range orig.Postings {
				var amount int64
				if err := store.DB().QueryRow(
					`SELECT amount FROM postings WHERE id = ?`, p.ID).Scan(&amount); err != nil {
					t.Fatalf("original posting %s missing after reversal: %v", p.ID, err)
				}
				if amount != int64(p.Amount) {
					t.Fatalf("original posting %s was mutated: %d != %d", p.ID, amount, p.Amount)
				}
			}

			// The reversed entry still shows in the statement (R12).
			page, err := l.Statement(ctx, b.ID, core.Page{Limit: 50})
			if err != nil {
				t.Fatalf("statement: %v", err)
			}
			seen := false
			for _, p := range page.Postings {
				if p.TransferID == orig.ID {
					seen = true
				}
			}
			if !seen {
				t.Fatalf("reversed entry vanished from the statement")
			}

			// The append-only triggers are still armed (R5).
			if _, err := store.DB().Exec(
				`UPDATE postings SET amount = 1 WHERE id = ?`, orig.Postings[0].ID); err == nil {
				t.Fatalf("UPDATE on postings was allowed; the append-only trigger is gone")
			}
			if _, err := store.DB().Exec(
				`DELETE FROM postings WHERE id = ?`, orig.Postings[0].ID); err == nil {
				t.Fatalf("DELETE on postings was allowed; the append-only trigger is gone")
			}
		})
	}
}

// R13 against real SQLite: a second reversal is a typed conflict and writes nothing.
func TestReverse_RealAdapter_AlreadyReversed(t *testing.T) {
	for _, source := range schemaSources {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			l, store := ledgerOn(t, source)
			_, _, orig := funded(t, l, 100, 30)

			if _, err := l.Reverse(ctx, "bob", orig.ID, "rev-1"); err != nil {
				t.Fatalf("first reverse: %v", err)
			}
			var before int64
			if err := store.DB().QueryRow(`SELECT COUNT(*) FROM postings`).Scan(&before); err != nil {
				t.Fatalf("count: %v", err)
			}

			_, err := l.Reverse(ctx, "bob", orig.ID, "rev-2")
			if !errors.Is(err, core.ErrAlreadyReversed) {
				t.Fatalf("second reverse: got %v, want ErrAlreadyReversed", err)
			}

			var after int64
			if err := store.DB().QueryRow(`SELECT COUNT(*) FROM postings`).Scan(&after); err != nil {
				t.Fatalf("count: %v", err)
			}
			if after != before {
				t.Fatalf("refused reversal wrote %d postings", after-before)
			}
		})
	}
}

// R4 against real SQLite: a replayed key returns the original reversal and the
// UNIQUE(owner, idempotency_key) constraint is never hit, because the engine
// short-circuits before the insert.
func TestReverse_RealAdapter_IdempotentReplay(t *testing.T) {
	for _, source := range schemaSources {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			l, store := ledgerOn(t, source)
			_, _, orig := funded(t, l, 100, 30)

			first, err := l.Reverse(ctx, "bob", orig.ID, "rev-1")
			if err != nil {
				t.Fatalf("first reverse: %v", err)
			}
			var before int64
			if err := store.DB().QueryRow(`SELECT COUNT(*) FROM postings`).Scan(&before); err != nil {
				t.Fatalf("count: %v", err)
			}

			second, err := l.Reverse(ctx, "bob", orig.ID, "rev-1")
			if err != nil {
				t.Fatalf("replay: %v", err)
			}
			if !second.Replayed {
				t.Fatalf("replay not flagged")
			}
			if second.Transfer.ID != first.Transfer.ID {
				t.Fatalf("replay returned a different entry")
			}

			var after int64
			if err := store.DB().QueryRow(`SELECT COUNT(*) FROM postings`).Scan(&after); err != nil {
				t.Fatalf("count: %v", err)
			}
			if after != before {
				t.Fatalf("replay posted again")
			}
		})
	}
}

// R7/R3 against real SQLite: an unauthorised reversal, and one the destination
// can no longer fund, are both refused before anything is written.
func TestReverse_RealAdapter_RefusalsWriteNothing(t *testing.T) {
	for _, source := range schemaSources {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			l, store := ledgerOn(t, source)
			a, b, orig := funded(t, l, 100, 30)

			var before int64
			if err := store.DB().QueryRow(`SELECT COUNT(*) FROM postings`).Scan(&before); err != nil {
				t.Fatalf("count: %v", err)
			}

			// R7: alice sent it, but the reversal debits bob.
			if _, err := l.Reverse(ctx, "alice", orig.ID, "rev-a"); !errors.Is(err, core.ErrUnauthorized) {
				t.Fatalf("got %v, want ErrUnauthorized", err)
			}

			// R3: bob spends the funds, so he can no longer cover the reversal.
			if _, err := l.Transfer(ctx, core.TransferCmd{
				Authorizer: "bob", From: b.ID, To: a.ID,
				Amount: 30, Currency: "USD", IdempotencyKey: "spend",
			}); err != nil {
				t.Fatalf("spend: %v", err)
			}
			if _, err := l.Reverse(ctx, "bob", orig.ID, "rev-b"); !errors.Is(err, core.ErrInsufficientFunds) {
				t.Fatalf("got %v, want ErrInsufficientFunds", err)
			}

			// Only the spend's two postings were added by the above.
			var after int64
			if err := store.DB().QueryRow(`SELECT COUNT(*) FROM postings`).Scan(&after); err != nil {
				t.Fatalf("count: %v", err)
			}
			if after != before+2 {
				t.Fatalf("refused reversals wrote postings: %d -> %d", before, after)
			}
		})
	}
}

// An unknown entry is a typed not-found from the real adapter, not a scan error.
func TestReverse_RealAdapter_UnknownEntry(t *testing.T) {
	for _, source := range schemaSources {
		t.Run(source, func(t *testing.T) {
			l, _ := ledgerOn(t, source)
			_, _, _ = funded(t, l, 100, 30)

			_, err := l.Reverse(context.Background(), "bob", "xfer_nope", "rev-1")
			if !errors.Is(err, core.ErrTransferNotFound) {
				t.Fatalf("got %v, want ErrTransferNotFound", err)
			}
		})
	}
}

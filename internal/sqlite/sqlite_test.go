package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Stephen-Kamau/ledger-wallet/core"
	"github.com/Stephen-Kamau/ledger-wallet/internal/sqlite"
)

// newLedger spins a fresh WAL DB in a temp dir with the real adapter wired into
// core.New. This proves the adapter and the engine agree against real SQLite, not a
// fake. Kofi's tests/ suite owns the heavy concurrency load; this is the adapter's
// own standalone proof.
func newLedger(t *testing.T) (core.Ledger, *sqlite.Store) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return core.New(store), store
}

func mustAcct(t *testing.T, l core.Ledger, owner, cur string, neg bool) core.Account {
	t.Helper()
	a, err := l.CreateAccount(context.Background(), core.CreateAccountCmd{
		Owner: core.PrincipalID(owner), Currency: core.Currency(cur), AllowNegative: neg,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return a
}

// A transfer posts balanced legs and balance is derived from the posting SUM.
func TestTransfer_BalancedAndDerived(t *testing.T) {
	l, store := newLedger(t)
	ctx := context.Background()

	sys := mustAcct(t, l, "system", "USD", true)
	alice := mustAcct(t, l, "alice", "USD", false)
	bob := mustAcct(t, l, "bob", "USD", false)

	// Mint 100 into alice from the system account (R2: minting is a transfer).
	if _, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "system", From: sys.ID, To: alice.ID,
		Amount: 100, Currency: "USD", IdempotencyKey: "mint",
	}); err != nil {
		t.Fatalf("mint: %v", err)
	}

	res, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "alice", From: alice.ID, To: bob.ID,
		Amount: 30, Currency: "USD", IdempotencyKey: "t1",
	})
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}

	// Balanced legs persisted.
	var sum core.Amount
	for _, p := range res.Transfer.Postings {
		sum += p.Amount
	}
	if sum != 0 || len(res.Transfer.Postings) != 2 {
		t.Fatalf("legs not balanced: sum=%d n=%d", sum, len(res.Transfer.Postings))
	}

	// Derived balances via SUM(postings).
	if bal, _ := l.Balance(ctx, alice.ID); bal != 70 {
		t.Fatalf("alice: want 70, got %d", bal)
	}
	if bal, _ := l.Balance(ctx, bob.ID); bal != 30 {
		t.Fatalf("bob: want 30, got %d", bal)
	}
	if bal, _ := l.Balance(ctx, sys.ID); bal != -100 {
		t.Fatalf("system: want -100, got %d", bal)
	}

	// Ledger-wide conservation (R2): everything nets to zero.
	var total int64
	if err := store.DB().QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount),0) FROM postings`).Scan(&total); err != nil {
		t.Fatalf("sum: %v", err)
	}
	if total != 0 {
		t.Fatalf("ledger not conserved: %d", total)
	}
}

// R3 enforced inside the write tx against the real DB.
func TestSQLite_InsufficientFunds(t *testing.T) {
	l, _ := newLedger(t)
	ctx := context.Background()
	sys := mustAcct(t, l, "system", "USD", true)
	alice := mustAcct(t, l, "alice", "USD", false)
	bob := mustAcct(t, l, "bob", "USD", false)
	mint(t, l, sys.ID, alice.ID, 10)

	_, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "alice", From: alice.ID, To: bob.ID,
		Amount: 11, Currency: "USD", IdempotencyKey: "over",
	})
	if err == nil || err.Error() != core.ErrInsufficientFunds.Error() {
		t.Fatalf("want ErrInsufficientFunds, got %v", err)
	}
}

// R4 replay against the real DB: no second posting, same transfer id.
func TestSQLite_IdempotentReplay(t *testing.T) {
	l, store := newLedger(t)
	ctx := context.Background()
	sys := mustAcct(t, l, "system", "USD", true)
	alice := mustAcct(t, l, "alice", "USD", false)
	bob := mustAcct(t, l, "bob", "USD", false)
	mint(t, l, sys.ID, alice.ID, 100)

	cmd := core.TransferCmd{Authorizer: "alice", From: alice.ID, To: bob.ID,
		Amount: 20, Currency: "USD", IdempotencyKey: "dup"}
	first, err := l.Transfer(ctx, cmd)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := l.Transfer(ctx, cmd)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !second.Replayed || second.Transfer.ID != first.Transfer.ID {
		t.Fatalf("replay mismatch: replayed=%v ids %s/%s", second.Replayed, first.Transfer.ID, second.Transfer.ID)
	}

	var transfers, postings int64
	store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM transfers WHERE idempotency_key='dup'`).Scan(&transfers)
	store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM postings`).Scan(&postings)
	if transfers != 1 {
		t.Fatalf("want 1 transfer row, got %d", transfers)
	}
	if postings != 4 { // mint (2) + the single dup transfer (2)
		t.Fatalf("want 4 postings, got %d", postings)
	}
}

// seq is assigned per account inside the write tx and ordered in statements (D-B1, R12).
func TestSQLite_SeqAndStatement(t *testing.T) {
	l, _ := newLedger(t)
	ctx := context.Background()
	sys := mustAcct(t, l, "system", "USD", true)
	alice := mustAcct(t, l, "alice", "USD", false)
	bob := mustAcct(t, l, "bob", "USD", false)
	mint(t, l, sys.ID, alice.ID, 100)
	for i, amt := range []core.Amount{5, 7, 9} {
		if _, err := l.Transfer(ctx, core.TransferCmd{
			Authorizer: "alice", From: alice.ID, To: bob.ID,
			Amount: amt, Currency: "USD", IdempotencyKey: "s" + string(rune('0'+i)),
		}); err != nil {
			t.Fatalf("transfer: %v", err)
		}
	}
	page, err := l.Statement(ctx, alice.ID, core.Page{Limit: 2})
	if err != nil {
		t.Fatalf("statement: %v", err)
	}
	if !page.HasMore || len(page.Postings) != 2 {
		t.Fatalf("page: hasMore=%v n=%d", page.HasMore, len(page.Postings))
	}
	// alice postings: seq 1 (mint credit), 2 (-5), 3 (-7), 4 (-9)
	if page.Postings[0].Seq != 1 || page.Postings[1].Seq != 2 {
		t.Fatalf("seq order wrong: %d, %d", page.Postings[0].Seq, page.Postings[1].Seq)
	}
	next, err := l.Statement(ctx, alice.ID, core.Page{Limit: 2, AfterSeq: page.NextAfterSeq})
	if err != nil {
		t.Fatalf("statement page 2: %v", err)
	}
	if len(next.Postings) != 2 || next.Postings[0].Seq != 3 {
		t.Fatalf("keyset page 2 wrong: n=%d first=%d", len(next.Postings), next.Postings[0].Seq)
	}
}

// R5: the append-only triggers refuse a direct UPDATE/DELETE on postings.
func TestSQLite_AppendOnlyTriggers(t *testing.T) {
	l, store := newLedger(t)
	ctx := context.Background()
	sys := mustAcct(t, l, "system", "USD", true)
	alice := mustAcct(t, l, "alice", "USD", false)
	bob := mustAcct(t, l, "bob", "USD", false)
	mint(t, l, sys.ID, alice.ID, 50)
	if _, err := l.Transfer(ctx, core.TransferCmd{Authorizer: "alice", From: alice.ID, To: bob.ID,
		Amount: 5, Currency: "USD", IdempotencyKey: "t"}); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `UPDATE postings SET amount = amount + 1`); err == nil {
		t.Fatal("UPDATE on postings should be refused by the trigger")
	}
	if _, err := store.DB().ExecContext(ctx, `DELETE FROM postings`); err == nil {
		t.Fatal("DELETE on postings should be refused by the trigger")
	}
}

// Read-side: a non-existent account is ErrAccountNotFound, distinct from a zero balance.
func TestSQLite_UnknownAccount(t *testing.T) {
	l, _ := newLedger(t)
	if _, err := l.Balance(context.Background(), "ghost"); err == nil ||
		err.Error() != core.ErrAccountNotFound.Error() {
		t.Fatalf("want ErrAccountNotFound, got %v", err)
	}
}

func mint(t *testing.T, l core.Ledger, sys, to core.AccountID, amt core.Amount) {
	t.Helper()
	if _, err := l.Transfer(context.Background(), core.TransferCmd{
		Authorizer: ownerOfSystem, From: sys, To: to,
		Amount: amt, Currency: "USD", IdempotencyKey: "mint-" + string(to),
	}); err != nil {
		t.Fatalf("mint: %v", err)
	}
}

const ownerOfSystem = core.PrincipalID("system")

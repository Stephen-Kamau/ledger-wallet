package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// Holds against a real WAL DB: a hold reduces available balance, a release restores
// it, an over-hold is refused, and the idempotency key is UNIQUE like a transfer's.
func TestSQLite_Holds(t *testing.T) {
	l, store := newLedger(t)
	ctx := context.Background()
	sys := mustAcct(t, l, "system", "USD", true)
	alice := mustAcct(t, l, "alice", "USD", false)
	mint(t, l, sys.ID, alice.ID, 100)

	// Place a 30 hold: posted balance unchanged, available drops to 70.
	h, err := l.PlaceHold(ctx, core.HoldCmd{
		Account: alice.ID, Authorizer: "alice", Amount: 30, Currency: "USD", IdempotencyKey: "h1",
	})
	if err != nil {
		t.Fatalf("place hold: %v", err)
	}
	if bal, _ := l.Balance(ctx, alice.ID); bal != 100 {
		t.Fatalf("posted balance must be unchanged by a hold: got %d", bal)
	}
	if avail, _ := l.AvailableBalance(ctx, alice.ID); avail != 70 {
		t.Fatalf("available: want 70, got %d", avail)
	}

	// The hold row is persisted and active.
	var active, amount int64
	if err := store.DB().QueryRowContext(ctx,
		`SELECT active, amount FROM holds WHERE id = ?`, string(h.ID)).Scan(&active, &amount); err != nil {
		t.Fatalf("read hold row: %v", err)
	}
	if active != 1 || amount != 30 {
		t.Fatalf("hold row: active=%d amount=%d, want 1/30", active, amount)
	}

	// Over-hold on the remaining available is refused and reserves nothing.
	if _, err := l.PlaceHold(ctx, core.HoldCmd{
		Account: alice.ID, Authorizer: "alice", Amount: 71, Currency: "USD", IdempotencyKey: "over",
	}); !errors.Is(err, core.ErrInsufficientFunds) {
		t.Fatalf("over-hold: want ErrInsufficientFunds, got %v", err)
	}

	// Release frees the funds: available back to 100 and the row is inactive.
	if err := l.ReleaseHold(ctx, "alice", h.ID); err != nil {
		t.Fatalf("release: %v", err)
	}
	if avail, _ := l.AvailableBalance(ctx, alice.ID); avail != 100 {
		t.Fatalf("available after release: want 100, got %d", avail)
	}
	store.DB().QueryRowContext(ctx, `SELECT active FROM holds WHERE id = ?`, string(h.ID)).Scan(&active)
	if active != 0 {
		t.Fatalf("released hold must be inactive, got active=%d", active)
	}

	// Releasing again is ErrHoldNotFound.
	if err := l.ReleaseHold(ctx, "alice", h.ID); !errors.Is(err, core.ErrHoldNotFound) {
		t.Fatalf("double release: want ErrHoldNotFound, got %v", err)
	}
}

// The holds idempotency key is UNIQUE per owner: a replay returns the original hold
// and inserts no second row.
func TestSQLite_HoldIdempotentReplay(t *testing.T) {
	l, store := newLedger(t)
	ctx := context.Background()
	sys := mustAcct(t, l, "system", "USD", true)
	alice := mustAcct(t, l, "alice", "USD", false)
	mint(t, l, sys.ID, alice.ID, 100)

	cmd := core.HoldCmd{Account: alice.ID, Authorizer: "alice", Amount: 20, Currency: "USD", IdempotencyKey: "dup"}
	first, err := l.PlaceHold(ctx, cmd)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := l.PlaceHold(ctx, cmd)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("replay returned a different hold: %s vs %s", first.ID, second.ID)
	}
	var n int64
	store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM holds WHERE idempotency_key = 'dup'`).Scan(&n)
	if n != 1 {
		t.Fatalf("want 1 hold row, got %d", n)
	}
	if avail, _ := l.AvailableBalance(ctx, alice.ID); avail != 80 {
		t.Fatalf("replay reserved twice: available %d, want 80", avail)
	}
}

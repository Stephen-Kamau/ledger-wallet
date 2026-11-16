package core

import (
	"context"
	"errors"
	"testing"
)

// H1: a hold reserves funds - it reduces AvailableBalance by its amount but leaves
// the posted Balance untouched (a hold posts nothing).
func TestHold_ReducesAvailable(t *testing.T) {
	l, _, _, a, _ := setup(t, 100)
	ctx := context.Background()

	h, err := l.PlaceHold(ctx, HoldCmd{
		Account: a.ID, Authorizer: "alice", Amount: 30, Currency: "USD", IdempotencyKey: "h1",
	})
	must(t, err)
	if !h.Active {
		t.Fatal("a fresh hold must be active")
	}
	if h.Amount != 30 || h.Account != a.ID {
		t.Fatalf("hold did not record its command: %+v", h)
	}

	if bal, _ := l.Balance(ctx, a.ID); bal != 100 {
		t.Fatalf("a hold must not move the posted balance: got %d", bal)
	}
	if avail, _ := l.AvailableBalance(ctx, a.ID); avail != 70 {
		t.Fatalf("available balance: want 70 (100-30), got %d", avail)
	}
}

// H2: releasing an active hold frees the funds - available returns to the posted
// balance.
func TestHold_ReleaseRestores(t *testing.T) {
	l, _, _, a, _ := setup(t, 100)
	ctx := context.Background()

	h, err := l.PlaceHold(ctx, HoldCmd{
		Account: a.ID, Authorizer: "alice", Amount: 40, Currency: "USD", IdempotencyKey: "h2",
	})
	must(t, err)
	if avail, _ := l.AvailableBalance(ctx, a.ID); avail != 60 {
		t.Fatalf("available after hold: want 60, got %d", avail)
	}

	must(t, l.ReleaseHold(ctx, "alice", h.ID))

	if avail, _ := l.AvailableBalance(ctx, a.ID); avail != 100 {
		t.Fatalf("available after release: want 100, got %d", avail)
	}
}

// H3: a hold that would drive available below zero on a normal account is rejected,
// and reserves nothing. Active holds count against the next hold.
func TestHold_OverHoldRejected(t *testing.T) {
	l, _, _, a, _ := setup(t, 100)
	ctx := context.Background()

	// A single hold above the whole balance is rejected.
	if _, err := l.PlaceHold(ctx, HoldCmd{
		Account: a.ID, Authorizer: "alice", Amount: 101, Currency: "USD", IdempotencyKey: "over1",
	}); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("want ErrInsufficientFunds, got %v", err)
	}
	if avail, _ := l.AvailableBalance(ctx, a.ID); avail != 100 {
		t.Fatalf("rejected hold must reserve nothing: available %d", avail)
	}

	// Two holds that each fit alone but together exceed the balance: the second is
	// rejected because the first reduced available.
	_, err := l.PlaceHold(ctx, HoldCmd{
		Account: a.ID, Authorizer: "alice", Amount: 60, Currency: "USD", IdempotencyKey: "h-a",
	})
	must(t, err)
	if _, err := l.PlaceHold(ctx, HoldCmd{
		Account: a.ID, Authorizer: "alice", Amount: 60, Currency: "USD", IdempotencyKey: "h-b",
	}); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("second over-hold: want ErrInsufficientFunds, got %v", err)
	}
	if avail, _ := l.AvailableBalance(ctx, a.ID); avail != 40 {
		t.Fatalf("available after one 60 hold: want 40, got %d", avail)
	}
}

// H4: a hold placed by a principal who does not own the account is rejected before
// any reservation (reuses R7 ownership / ErrUnauthorized).
func TestHold_UnauthorizedRejected(t *testing.T) {
	l, _, _, a, _ := setup(t, 100)
	ctx := context.Background()
	if _, err := l.PlaceHold(ctx, HoldCmd{
		Account: a.ID, Authorizer: "bob", Amount: 10, Currency: "USD", IdempotencyKey: "steal",
	}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	if avail, _ := l.AvailableBalance(ctx, a.ID); avail != 100 {
		t.Fatalf("unauthorized hold reserved funds: available %d", avail)
	}
}

// H5: replaying a hold's idempotency key returns the original hold and reserves
// nothing new; a key reused with different params is a conflict.
func TestHold_IdempotentReplay(t *testing.T) {
	l, _, _, a, _ := setup(t, 100)
	ctx := context.Background()
	cmd := HoldCmd{Account: a.ID, Authorizer: "alice", Amount: 25, Currency: "USD", IdempotencyKey: "dup"}

	first, err := l.PlaceHold(ctx, cmd)
	must(t, err)
	second, err := l.PlaceHold(ctx, cmd)
	must(t, err)
	if second.ID != first.ID {
		t.Fatalf("replay returned a different hold: %s vs %s", first.ID, second.ID)
	}
	if avail, _ := l.AvailableBalance(ctx, a.ID); avail != 75 {
		t.Fatalf("replay reserved twice: available %d, want 75", avail)
	}

	conflict := cmd
	conflict.Amount = 26
	if _, err := l.PlaceHold(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("want ErrIdempotencyConflict, got %v", err)
	}
}

// H6: releasing an unknown or already-released hold is a typed ErrHoldNotFound.
func TestReleaseHold_NotFound(t *testing.T) {
	l, _, _, a, _ := setup(t, 100)
	ctx := context.Background()

	if err := l.ReleaseHold(ctx, "alice", "hold_nope"); !errors.Is(err, ErrHoldNotFound) {
		t.Fatalf("unknown hold: want ErrHoldNotFound, got %v", err)
	}

	h, err := l.PlaceHold(ctx, HoldCmd{
		Account: a.ID, Authorizer: "alice", Amount: 10, Currency: "USD", IdempotencyKey: "rel",
	})
	must(t, err)
	must(t, l.ReleaseHold(ctx, "alice", h.ID))
	// Second release of the same hold is ErrHoldNotFound (no longer active).
	if err := l.ReleaseHold(ctx, "alice", h.ID); !errors.Is(err, ErrHoldNotFound) {
		t.Fatalf("double release: want ErrHoldNotFound, got %v", err)
	}
}

// H7: releasing a hold on an account the authorizer does not own is ErrUnauthorized,
// and the hold stays active.
func TestReleaseHold_Unauthorized(t *testing.T) {
	l, _, _, a, _ := setup(t, 100)
	ctx := context.Background()
	h, err := l.PlaceHold(ctx, HoldCmd{
		Account: a.ID, Authorizer: "alice", Amount: 10, Currency: "USD", IdempotencyKey: "rel2",
	})
	must(t, err)
	if err := l.ReleaseHold(ctx, "bob", h.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	if avail, _ := l.AvailableBalance(ctx, a.ID); avail != 90 {
		t.Fatalf("hold must survive an unauthorized release: available %d, want 90", avail)
	}
}

// H8: currency disagreement between the command and the account is rejected.
func TestHold_CurrencyMismatch(t *testing.T) {
	l, _, _, a, _ := setup(t, 100)
	ctx := context.Background()
	if _, err := l.PlaceHold(ctx, HoldCmd{
		Account: a.ID, Authorizer: "alice", Amount: 10, Currency: "GBP", IdempotencyKey: "cm",
	}); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("want ErrCurrencyMismatch, got %v", err)
	}
}

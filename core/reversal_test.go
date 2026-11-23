package core

import (
	"context"
	"errors"
	"testing"
)

// reversible sets up alice -> bob and returns the original transfer, so each test
// starts from a posted entry that is a candidate for reversal.
func reversible(t *testing.T, fund, move Amount) (Ledger, *fakeStore, Account, Account, Transfer) {
	t.Helper()
	l, store, _, a, b := setup(t, fund)
	res, err := l.Transfer(context.Background(), TransferCmd{
		Authorizer: "alice", From: a.ID, To: b.ID,
		Amount: move, Currency: "USD", IdempotencyKey: "orig",
	})
	must(t, err)
	return l, store, a, b, res.Transfer
}

// R1: the compensating entry's own postings sum to exactly zero.
func TestReverse_R1_CompensatingPostingsNetToZero(t *testing.T) {
	l, _, _, _, orig := reversible(t, 100, 30)

	res, err := l.Reverse(context.Background(), "bob", orig.ID, "rev-1")
	must(t, err)

	var sum Amount
	for _, p := range res.Transfer.Postings {
		sum += p.Amount
	}
	if sum != 0 {
		t.Fatalf("reversal postings did not net to zero: got %d", sum)
	}
	if len(res.Transfer.Postings) != len(orig.Postings) {
		t.Fatalf("reversal should mirror every original leg: got %d want %d",
			len(res.Transfer.Postings), len(orig.Postings))
	}
}

// The reversal MIRRORS the original postings: same accounts, negated amounts.
func TestReverse_MirrorsOriginalPostings(t *testing.T) {
	l, _, _, _, orig := reversible(t, 100, 30)

	res, err := l.Reverse(context.Background(), "bob", orig.ID, "rev-1")
	must(t, err)

	mirrored := map[AccountID]Amount{}
	for _, p := range res.Transfer.Postings {
		mirrored[p.Account] += p.Amount
	}
	for _, p := range orig.Postings {
		if got := mirrored[p.Account]; got != -p.Amount {
			t.Fatalf("account %s: mirrored amount %d, want %d", p.Account, got, -p.Amount)
		}
	}
}

// R2: conservation - reversing restores every balance to its pre-transfer value and
// leaves the ledger-wide sum unchanged.
func TestReverse_R2_ConservationRestoresBalances(t *testing.T) {
	l, store, a, b, orig := reversible(t, 100, 30)
	ctx := context.Background()

	before := ledgerSum(store)
	balA, err := l.Balance(ctx, a.ID)
	must(t, err)
	balB, err := l.Balance(ctx, b.ID)
	must(t, err)
	if balA != 70 || balB != 30 {
		t.Fatalf("pre-reversal balances wrong: a=%d b=%d", balA, balB)
	}

	if _, err := l.Reverse(ctx, "bob", orig.ID, "rev-1"); err != nil {
		t.Fatalf("reverse: %v", err)
	}

	balA, err = l.Balance(ctx, a.ID)
	must(t, err)
	balB, err = l.Balance(ctx, b.ID)
	must(t, err)
	if balA != 100 || balB != 0 {
		t.Fatalf("post-reversal balances not restored: a=%d b=%d", balA, balB)
	}
	if after := ledgerSum(store); after != before {
		t.Fatalf("conservation broken: sum %d -> %d", before, after)
	}
}

// R5: the reversed entry is still there, and still shows in the statement. The
// reversal ADDS postings; it never removes or edits the originals.
func TestReverse_R5_OriginalEntryAndPostingsSurvive(t *testing.T) {
	l, store, _, b, orig := reversible(t, 100, 30)
	ctx := context.Background()

	postingsBefore := len(store.postings)

	if _, err := l.Reverse(ctx, "bob", orig.ID, "rev-1"); err != nil {
		t.Fatalf("reverse: %v", err)
	}

	if len(store.postings) != postingsBefore+len(orig.Postings) {
		t.Fatalf("expected %d postings after reversal, got %d",
			postingsBefore+len(orig.Postings), len(store.postings))
	}
	// Every original posting is still present, byte for byte.
	for _, want := range orig.Postings {
		found := false
		for _, got := range store.postings {
			if got.ID == want.ID && got.Amount == want.Amount && got.Account == want.Account {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("original posting %s was mutated or removed", want.ID)
		}
	}

	// And it still shows in the statement.
	page, err := l.Statement(ctx, b.ID, Page{Limit: 50})
	must(t, err)
	seen := false
	for _, p := range page.Postings {
		if p.TransferID == orig.ID {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("reversed entry no longer appears in the statement")
	}
}

// R13: reversing an already-reversed entry is a typed conflict, not a second
// compensating entry.
func TestReverse_R13_AlreadyReversedIsTypedConflict(t *testing.T) {
	l, store, _, _, orig := reversible(t, 100, 30)
	ctx := context.Background()

	if _, err := l.Reverse(ctx, "bob", orig.ID, "rev-1"); err != nil {
		t.Fatalf("first reverse: %v", err)
	}
	countAfterFirst := len(store.postings)

	_, err := l.Reverse(ctx, "bob", orig.ID, "rev-2")
	if !errors.Is(err, ErrAlreadyReversed) {
		t.Fatalf("second reverse: got %v, want ErrAlreadyReversed", err)
	}
	if len(store.postings) != countAfterFirst {
		t.Fatalf("a rejected second reversal still wrote postings")
	}
}

// R13: an unknown entry id is a typed not-found, never a bare failure.
func TestReverse_R13_UnknownEntryIsTypedNotFound(t *testing.T) {
	l, _, _, _, _ := reversible(t, 100, 30)

	_, err := l.Reverse(context.Background(), "bob", "xfer_does_not_exist", "rev-1")
	if !errors.Is(err, ErrTransferNotFound) {
		t.Fatalf("got %v, want ErrTransferNotFound", err)
	}
}

// R7: the reversal DEBITS the original destination, so the destination's owner must
// authorise it. Anyone else is rejected before anything is written.
func TestReverse_R7_OnlyDebitedOwnerMayReverse(t *testing.T) {
	l, store, _, _, orig := reversible(t, 100, 30)
	before := len(store.postings)

	// alice sent the original, but the reversal debits bob - alice may not authorise it.
	_, err := l.Reverse(context.Background(), "alice", orig.ID, "rev-1")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
	if len(store.postings) != before {
		t.Fatalf("unauthorised reversal wrote postings")
	}
}

// R3: the reversal is a real debit of the destination, so it is refused when the
// destination has already spent the funds and may not go negative.
func TestReverse_R3_InsufficientFundsWhenDestinationSpent(t *testing.T) {
	l, store, a, b, orig := reversible(t, 100, 30)
	ctx := context.Background()

	// bob spends what he received, so he can no longer fund the reversal.
	_, err := l.Transfer(ctx, TransferCmd{
		Authorizer: "bob", From: b.ID, To: a.ID,
		Amount: 30, Currency: "USD", IdempotencyKey: "spend",
	})
	must(t, err)

	before := len(store.postings)
	_, err = l.Reverse(ctx, "bob", orig.ID, "rev-1")
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("got %v, want ErrInsufficientFunds", err)
	}
	if len(store.postings) != before {
		t.Fatalf("refused reversal still wrote postings")
	}
}

// R4: replaying the same idempotency key returns the original reversal and does not
// post a second time.
func TestReverse_R4_IdempotentReplay(t *testing.T) {
	l, store, _, _, orig := reversible(t, 100, 30)
	ctx := context.Background()

	first, err := l.Reverse(ctx, "bob", orig.ID, "rev-1")
	must(t, err)
	if first.Replayed {
		t.Fatalf("first reversal reported as a replay")
	}
	count := len(store.postings)

	second, err := l.Reverse(ctx, "bob", orig.ID, "rev-1")
	must(t, err)
	if !second.Replayed {
		t.Fatalf("replay not flagged")
	}
	if second.Transfer.ID != first.Transfer.ID {
		t.Fatalf("replay returned a different transfer: %s vs %s",
			second.Transfer.ID, first.Transfer.ID)
	}
	if len(store.postings) != count {
		t.Fatalf("replay posted a second time")
	}
}

// R4: the same key reused for a DIFFERENT entry is a conflict.
func TestReverse_R4_KeyReuseAcrossEntriesConflicts(t *testing.T) {
	l, _, a, b, orig := reversible(t, 100, 30)
	ctx := context.Background()

	second, err := l.Transfer(ctx, TransferCmd{
		Authorizer: "alice", From: a.ID, To: b.ID,
		Amount: 10, Currency: "USD", IdempotencyKey: "orig-2",
	})
	must(t, err)

	if _, err := l.Reverse(ctx, "bob", orig.ID, "rev-1"); err != nil {
		t.Fatalf("first reverse: %v", err)
	}
	_, err = l.Reverse(ctx, "bob", second.Transfer.ID, "rev-1")
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("got %v, want ErrIdempotencyConflict", err)
	}
}

// The reversal of a reversal is itself a legitimate compensating entry: it is a
// different entry, so it is NOT the already-reversed conflict, and it restores the
// original effect.
func TestReverse_ReversalOfAReversalIsAllowed(t *testing.T) {
	l, store, a, b, orig := reversible(t, 100, 30)
	ctx := context.Background()

	rev, err := l.Reverse(ctx, "bob", orig.ID, "rev-1")
	must(t, err)

	before := ledgerSum(store)
	// The first reversal credited alice and debited bob; undoing it debits alice.
	if _, err := l.Reverse(ctx, "alice", rev.Transfer.ID, "rev-2"); err != nil {
		t.Fatalf("reversing the reversal: %v", err)
	}

	balA, err := l.Balance(ctx, a.ID)
	must(t, err)
	balB, err := l.Balance(ctx, b.ID)
	must(t, err)
	if balA != 70 || balB != 30 {
		t.Fatalf("double reversal did not restore the original effect: a=%d b=%d", balA, balB)
	}
	if after := ledgerSum(store); after != before {
		t.Fatalf("conservation broken: sum %d -> %d", before, after)
	}
}

// ledgerSum totals every posting in the store - the quantity R2 holds constant.
func ledgerSum(s *fakeStore) Amount {
	var sum Amount
	for _, p := range s.postings {
		sum += p.Amount
	}
	return sum
}

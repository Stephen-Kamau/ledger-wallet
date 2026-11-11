package core

import (
	"context"
	"errors"
	"testing"
)

// setup mints a system (AllowNegative) account plus two normal accounts for one
// owner in USD, and funds `fund` units into acctA via a transfer from system (R2:
// minting IS a transfer from the AllowNegative account).
func setup(t *testing.T, fund Amount) (Ledger, *fakeStore, Account, Account, Account) {
	t.Helper()
	store := newFakeStore()
	l := New(store)
	ctx := context.Background()

	sys, err := l.CreateAccount(ctx, CreateAccountCmd{Owner: "system", Currency: "USD", AllowNegative: true})
	must(t, err)
	a, err := l.CreateAccount(ctx, CreateAccountCmd{Owner: "alice", Currency: "USD"})
	must(t, err)
	b, err := l.CreateAccount(ctx, CreateAccountCmd{Owner: "bob", Currency: "USD"})
	must(t, err)

	if fund > 0 {
		_, err = l.Transfer(ctx, TransferCmd{
			Authorizer: "system", From: sys.ID, To: a.ID,
			Amount: fund, Currency: "USD", IdempotencyKey: "mint-a",
		})
		must(t, err)
	}
	return l, store, sys, a, b
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// R1: a transfer's postings sum to zero; a zero/negative magnitude is unbalanced.
func TestR1_BalancedPostings(t *testing.T) {
	l, store, _, a, b := setup(t, 100)
	ctx := context.Background()

	res, err := l.Transfer(ctx, TransferCmd{
		Authorizer: "alice", From: a.ID, To: b.ID,
		Amount: 30, Currency: "USD", IdempotencyKey: "k1",
	})
	must(t, err)

	var sum Amount
	for _, p := range res.Transfer.Postings {
		sum += p.Amount
	}
	if sum != 0 {
		t.Fatalf("postings did not net to zero: got %d", sum)
	}
	if len(res.Transfer.Postings) != 2 {
		t.Fatalf("want 2 postings, got %d", len(res.Transfer.Postings))
	}
	_ = store

	for _, bad := range []Amount{0, -5} {
		if _, err := l.Transfer(ctx, TransferCmd{
			Authorizer: "alice", From: a.ID, To: b.ID,
			Amount: bad, Currency: "USD", IdempotencyKey: "bad",
		}); !errors.Is(err, ErrUnbalanced) {
			t.Fatalf("amount %d: want ErrUnbalanced, got %v", bad, err)
		}
	}
}

// R2: conservation - the whole ledger sums to zero after any sequence of transfers.
func TestR2_Conservation(t *testing.T) {
	l, store, _, a, b := setup(t, 100)
	ctx := context.Background()
	for i, amt := range []Amount{10, 20, 5} {
		_, err := l.Transfer(ctx, TransferCmd{
			Authorizer: "alice", From: a.ID, To: b.ID,
			Amount: amt, Currency: "USD", IdempotencyKey: string(rune('a' + i)),
		})
		must(t, err)
	}
	var total Amount
	for _, p := range store.postings {
		total += p.Amount
	}
	if total != 0 {
		t.Fatalf("ledger not conserved: sum of all postings = %d", total)
	}
}

// R3: a debit that would take a normal account negative is rejected; the balance is
// unchanged. The AllowNegative system account may go negative (exercised by minting).
func TestR3_NoNegativeBalance(t *testing.T) {
	l, _, sys, a, b := setup(t, 50)
	ctx := context.Background()

	_, err := l.Transfer(ctx, TransferCmd{
		Authorizer: "alice", From: a.ID, To: b.ID,
		Amount: 51, Currency: "USD", IdempotencyKey: "overdraw",
	})
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("want ErrInsufficientFunds, got %v", err)
	}
	bal, _ := l.Balance(ctx, a.ID)
	if bal != 50 {
		t.Fatalf("balance changed on rejected transfer: got %d", bal)
	}

	sysBal, _ := l.Balance(ctx, sys.ID)
	if sysBal != -50 { // system went negative by the minted amount - allowed
		t.Fatalf("system balance: want -50, got %d", sysBal)
	}
}

// R4: replaying an idempotency key returns the original result with Replayed=true and
// posts nothing new; reusing a key with different params is a conflict.
func TestR4_Idempotency(t *testing.T) {
	l, store, _, a, b := setup(t, 100)
	ctx := context.Background()

	cmd := TransferCmd{Authorizer: "alice", From: a.ID, To: b.ID,
		Amount: 40, Currency: "USD", IdempotencyKey: "same"}

	first, err := l.Transfer(ctx, cmd)
	must(t, err)
	if first.Replayed {
		t.Fatal("first call must not be a replay")
	}
	postingsAfterFirst := len(store.postings)

	second, err := l.Transfer(ctx, cmd)
	must(t, err)
	if !second.Replayed {
		t.Fatal("second call with same key must be a replay")
	}
	if second.Transfer.ID != first.Transfer.ID {
		t.Fatal("replay returned a different transfer")
	}
	if len(store.postings) != postingsAfterFirst {
		t.Fatalf("replay posted again: %d -> %d", postingsAfterFirst, len(store.postings))
	}
	if balB, _ := l.Balance(ctx, b.ID); balB != 40 {
		t.Fatalf("replay moved money twice: bob=%d", balB)
	}

	conflict := cmd
	conflict.Amount = 41
	if _, err := l.Transfer(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("want ErrIdempotencyConflict, got %v", err)
	}
}

// R5: the Tx port exposes no update/delete of postings - append-only is structural.
// This test documents the structural guarantee by asserting the engine only grows
// the posting log across transfers.
func TestR5_AppendOnly(t *testing.T) {
	l, store, _, a, b := setup(t, 100)
	ctx := context.Background()
	before := len(store.postings)
	_, err := l.Transfer(ctx, TransferCmd{Authorizer: "alice", From: a.ID, To: b.ID,
		Amount: 10, Currency: "USD", IdempotencyKey: "append"})
	must(t, err)
	if len(store.postings) != before+2 {
		t.Fatalf("expected 2 appended postings, got %d", len(store.postings)-before)
	}
}

// R7: a transfer debiting an account not owned by the authorizer is rejected before
// any write.
func TestR7_AuthorisedDebit(t *testing.T) {
	l, store, _, a, b := setup(t, 100)
	ctx := context.Background()
	before := len(store.postings)

	_, err := l.Transfer(ctx, TransferCmd{
		Authorizer: "bob", From: a.ID, To: b.ID, // bob does not own a (alice's)
		Amount: 10, Currency: "USD", IdempotencyKey: "steal",
	})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	if len(store.postings) != before {
		t.Fatal("unauthorized transfer wrote postings")
	}
}

// R8: all legs and both accounts must share one currency.
func TestR8_SingleCurrency(t *testing.T) {
	store := newFakeStore()
	l := New(store)
	ctx := context.Background()
	usd, err := l.CreateAccount(ctx, CreateAccountCmd{Owner: "alice", Currency: "USD"})
	must(t, err)
	gbp, err := l.CreateAccount(ctx, CreateAccountCmd{Owner: "alice", Currency: "GBP"})
	must(t, err)

	// mismatched account currencies
	if _, err := l.Transfer(ctx, TransferCmd{
		Authorizer: "alice", From: usd.ID, To: gbp.ID,
		Amount: 10, Currency: "USD", IdempotencyKey: "x1",
	}); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("want ErrCurrencyMismatch for cross-currency accounts, got %v", err)
	}
	// cmd currency disagreeing with the accounts
	usd2, err := l.CreateAccount(ctx, CreateAccountCmd{Owner: "alice", Currency: "USD"})
	must(t, err)
	if _, err := l.Transfer(ctx, TransferCmd{
		Authorizer: "alice", From: usd.ID, To: usd2.ID,
		Amount: 10, Currency: "GBP", IdempotencyKey: "x2",
	}); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("want ErrCurrencyMismatch for cmd currency mismatch, got %v", err)
	}
}

// Account-not-found surfaces on an unknown account in a transfer.
func TestTransfer_AccountNotFound(t *testing.T) {
	l, _, _, a, _ := setup(t, 100)
	ctx := context.Background()
	if _, err := l.Transfer(ctx, TransferCmd{
		Authorizer: "alice", From: a.ID, To: "nope",
		Amount: 5, Currency: "USD", IdempotencyKey: "nf",
	}); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("want ErrAccountNotFound, got %v", err)
	}
}

// R6: CreateAccount requires an owner and a currency.
func TestR6_CreateAccountValidation(t *testing.T) {
	l := New(newFakeStore())
	ctx := context.Background()
	for _, cmd := range []CreateAccountCmd{
		{Owner: "", Currency: "USD"},
		{Owner: "alice", Currency: ""},
	} {
		if _, err := l.CreateAccount(ctx, cmd); !errors.Is(err, ErrInvalidAccount) {
			t.Fatalf("want ErrInvalidAccount for %+v, got %v", cmd, err)
		}
	}
}

// R20/D-C1: AccountsByOwner returns only the principal's accounts.
func TestAccountsByOwner(t *testing.T) {
	l, _, _, a, b := setup(t, 0)
	ctx := context.Background()
	alice, err := l.AccountsByOwner(ctx, "alice")
	must(t, err)
	if len(alice) != 1 || alice[0].ID != a.ID {
		t.Fatalf("alice should own exactly account a, got %+v", alice)
	}
	bob, err := l.AccountsByOwner(ctx, "bob")
	must(t, err)
	if len(bob) != 1 || bob[0].ID != b.ID {
		t.Fatalf("bob should own exactly account b, got %+v", bob)
	}
}

// Balance is derived from postings, not stored: it tracks the posting log exactly.
func TestBalance_Derived(t *testing.T) {
	l, _, _, a, b := setup(t, 100)
	ctx := context.Background()
	_, err := l.Transfer(ctx, TransferCmd{Authorizer: "alice", From: a.ID, To: b.ID,
		Amount: 25, Currency: "USD", IdempotencyKey: "d1"})
	must(t, err)
	if bal, _ := l.Balance(ctx, a.ID); bal != 75 {
		t.Fatalf("alice derived balance: want 75, got %d", bal)
	}
	if bal, _ := l.Balance(ctx, b.ID); bal != 25 {
		t.Fatalf("bob derived balance: want 25, got %d", bal)
	}
}

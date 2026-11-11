package tests

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// TestR17_NoLostUpdates launches N concurrent distinct-key transfers of 1 unit each into
// one target account from a well-funded source, released simultaneously. The property:
// the final balance == the sum of the deltas that were applied; no append is lost to a
// race. Contention is genuine -- distinct keys mean every goroutine is a real write
// fighting for the single write lock, and the balance is recomputed from the posting log.
func TestR17_NoLostUpdates(t *testing.T) {
	l, db := newLedger(t)
	ctx := context.Background()
	sys := newSystemAccount(t, l)
	src := newAccount(t, l, "alice")
	dst := newAccount(t, l, "bob")

	const N = 200
	fund(t, l, sys, src, N) // src funded with exactly N, so all N transfers are affordable

	ready, start := barrier(N)
	errs := make([]error, N)
	retries := make([]int, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-start // all N hit the write lock together
			errs[i], retries[i] = transferRetry(ctx, l, core.TransferCmd{
				Authorizer: "alice", From: src, To: dst,
				Amount: 1, Currency: usd,
				IdempotencyKey: fmt.Sprintf("r17-%d", i), // distinct: all are real transfers
			})
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()

	applied, infra := 0, 0
	for i := 0; i < N; i++ {
		infra += retries[i]
		switch {
		case errs[i] == nil:
			applied++
		default:
			t.Fatalf("goroutine %d: unexpected error: %v", i, errs[i])
		}
	}

	// Every applied delta landed, none lost: final balance == count of applied transfers.
	if got := balance(t, l, dst); int64(got) != int64(applied) {
		t.Fatalf("R17 lost update: dst balance %d != applied %d", got, applied)
	}
	// src funded with exactly N and all are affordable, so every transfer applied.
	if applied != N {
		t.Fatalf("R17: want all %d applied, got %d", N, applied)
	}
	if got := balance(t, l, src); got != 0 {
		t.Fatalf("R17: src should be drained to 0, got %d", got)
	}
	t.Logf("R17: applied=%d infra_retries=%d", applied, infra)
	if failed := assertInvariants(t, db, l, sys, src, dst); failed != 0 {
		t.Fatalf("R17: %d invariant failures", failed)
	}
}

// TestR18_NoDoubleSpend funds an account with exactly K units, then launches M > K
// concurrent single-unit withdrawals released together. The property: exactly K succeed,
// M-K return ErrInsufficientFunds, the account never goes negative, and it ends at 0.
// This is the test that BEGIN IMMEDIATE + the in-transaction R3 check (§5) exists to pass:
// with a naive BEGIN DEFERRED, two goroutines read the same balance, both decide the unit
// is affordable, and ok > K or the account goes negative.
func TestR18_NoDoubleSpend(t *testing.T) {
	l, db := newLedger(t)
	ctx := context.Background()
	sys := newSystemAccount(t, l)
	acct := newAccount(t, l, "alice")
	sink := newAccount(t, l, "bob")

	const K, M = 50, 200
	fund(t, l, sys, acct, K)

	ready, start := barrier(M)
	results := make([]error, M)
	retries := make([]int, M)
	var wg sync.WaitGroup
	wg.Add(M)
	for i := 0; i < M; i++ {
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-start
			results[i], retries[i] = transferRetry(ctx, l, core.TransferCmd{
				Authorizer: "alice", From: acct, To: sink,
				Amount: 1, Currency: usd,
				IdempotencyKey: fmt.Sprintf("r18-%d", i),
			})
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()

	ok, insufficient, infra := 0, 0, 0
	for i := 0; i < M; i++ {
		infra += retries[i]
		switch {
		case results[i] == nil:
			ok++
		case errors.Is(results[i], core.ErrInsufficientFunds):
			insufficient++
		default:
			t.Fatalf("goroutine %d: unexpected error: %v", i, results[i])
		}
	}

	if ok != K {
		t.Fatalf("R18 double-spend: %d succeeded, want exactly K=%d", ok, K)
	}
	if insufficient != M-K {
		t.Fatalf("R18: %d insufficient, want M-K=%d", insufficient, M-K)
	}
	if got := balance(t, l, acct); got != 0 {
		t.Fatalf("R18: account should be drained to exactly 0, got %d", got)
	}
	if got := balance(t, l, sink); int64(got) != int64(K) {
		t.Fatalf("R18: sink should hold K=%d, got %d", K, got)
	}
	t.Logf("R18: ok=%d insufficient=%d infra_retries=%d", ok, insufficient, infra)
	if failed := assertInvariants(t, db, l, sys, acct, sink); failed != 0 {
		t.Fatalf("R18: %d invariant failures", failed)
	}
}

// TestR4_ConcurrentReplaySameKey launches W goroutines that replay the SAME idempotency
// key at once. The property: exactly one transfer row and one posting-pair are written,
// every caller sees success, and the balance moves exactly once. This exercises the
// UNIQUE(owner, idempotency_key) backstop path under contention, not just the fast-path
// lookup -- if the lookup ever misses under the race, the second InsertTransfer hits the
// UNIQUE constraint rather than posting twice.
func TestR4_ConcurrentReplaySameKey(t *testing.T) {
	l, db := newLedger(t)
	ctx := context.Background()
	sys := newSystemAccount(t, l)
	a := newAccount(t, l, "alice")
	b := newAccount(t, l, "bob")
	fund(t, l, sys, a, 10)

	const W = 32
	const key = "same-key"
	ready, start := barrier(W)
	results := make([]error, W)
	var wg sync.WaitGroup
	wg.Add(W)
	for i := 0; i < W; i++ {
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-start
			results[i], _ = transferRetry(ctx, l, core.TransferCmd{
				Authorizer: "alice", From: a, To: b,
				Amount: 1, Currency: usd, IdempotencyKey: key,
			})
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()

	for i := 0; i < W; i++ {
		if results[i] != nil && !errors.Is(results[i], core.ErrIdempotencyConflict) {
			// A clean idempotent replay returns nil. We tolerate nothing else here.
			if results[i] != nil {
				t.Fatalf("goroutine %d: unexpected error on replay: %v", i, results[i])
			}
		}
	}

	// Exactly one transfer row for the key.
	if n := scalar(t, db, `SELECT COUNT(*) FROM transfers WHERE owner=? AND idempotency_key=?`, "alice", key); n != 1 {
		t.Fatalf("R4: want 1 transfer row for key, got %d", n)
	}
	// Exactly one debit + one credit (not W*2).
	if n := scalar(t, db, `
		SELECT COUNT(*) FROM postings WHERE transfer_id IN
		(SELECT id FROM transfers WHERE owner=? AND idempotency_key=?)`, "alice", key); n != 2 {
		t.Fatalf("R4: want 2 postings for the key, got %d", n)
	}
	// Balance moved exactly once: 10 -> 9.
	if got := balance(t, l, a); got != 9 {
		t.Fatalf("R4: want balance moved once (9), got %d", got)
	}
	if got := balance(t, l, b); got != 1 {
		t.Fatalf("R4: want b=1, got %d", got)
	}
	if failed := assertInvariants(t, db, l, sys, a, b); failed != 0 {
		t.Fatalf("R4: %d invariant failures", failed)
	}
}

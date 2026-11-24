package tests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// This file proves the property migration 0004_reversals exists for: an entry can be
// reversed AT MOST ONCE, and that is guaranteed by the DATABASE, not by a Go branch.
//
// The tests are written so that nothing BUT the 0004 constraint can make them pass.
// Every goroutine uses a DISTINCT idempotency key, so the R4 transfer constraint
// (UNIQUE (owner, idempotency_key)) cannot be what prevents the second compensating
// entry -- drop UNIQUE (original_transfer_id) from 0004 and TestR18_... posts twice.

// --- the reversal operation, claim then settle -----------------------------------
//
// THESE HELPERS ARE NOT THE PRODUCTION CONTRACT. Read `core.Reverse` for that.
//
// Production reverses an entry by DERIVING the compensating entry's id as
// `rev_<entryID>`, so "at most one reversal per entry" falls out of the transfers
// PRIMARY KEY, inside core's single write transaction, with no schema change and no
// second schema source. The `reversals` table these helpers drive is deliberately
// UNWIRED by Orchestrator decision: nothing in core, api, internal or cmd reads or
// writes it. It is kept because keeping the two schemas APART is precisely what
// tests/bootpath_test.go detects drift on -- wiring the table would cost that guard.
//
// So what these helpers prove is narrower than it looks, and worth being exact about:
// they prove the GUARANTEE 0004 carries is real and enforced by the database -- that a
// UNIQUE constraint, not a Go branch, is what survives 32 racing goroutines. They do
// not describe how the running system reverses anything.
//
// The claim-then-settle shape exists because this stream owns no seam inside core's
// write transaction: take the claim (which excludes every other reverser), post the
// compensating transfer through core.Ledger.Transfer, then settle. core.Reverse needs
// none of that -- it has one transaction and therefore no window between the two.

// isUniqueViolation reports whether err is a UNIQUE/PRIMARY KEY constraint refusal --
// an INVARIANT outcome (someone else reversed this entry first), never infrastructure.
// Kept distinct from isInfraBusy for the same reason transferRetry does: a busy lock is
// retried, a constraint refusal is a result.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique constraint failed") ||
		strings.Contains(s, "constraint failed: unique")
}

// claimReversal attempts the single claim on originalTransferID. It returns true when
// this caller won the claim, false when the database refused it because the entry is
// already claimed, and an error for anything else. Busy/locked is retried, never folded
// into the refusal count.
func claimReversal(ctx context.Context, db *sql.DB, id, originalTransferID string, owner core.PrincipalID) (bool, error) {
	var err error
	for attempt := 0; attempt < 50; attempt++ {
		_, err = db.ExecContext(ctx,
			`INSERT INTO reversals(id, original_transfer_id, owner, reason, created_at)
			 VALUES(?, ?, ?, ?, ?)`,
			id, originalTransferID, string(owner), "test", time.Now().UTC().Format(time.RFC3339Nano))
		switch {
		case err == nil:
			return true, nil
		case isUniqueViolation(err):
			return false, nil // already reversed -- the guarantee firing
		case isInfraBusy(err):
			continue
		default:
			return false, err
		}
	}
	return false, err
}

// settleReversal records the compensating transfer against a won claim.
func settleReversal(ctx context.Context, db *sql.DB, id, reversalTransferID string) error {
	var err error
	for attempt := 0; attempt < 50; attempt++ {
		_, err = db.ExecContext(ctx,
			`UPDATE reversals SET reversal_transfer_id = ?, settled_at = ? WHERE id = ?`,
			reversalTransferID, time.Now().UTC().Format(time.RFC3339Nano), id)
		if !isInfraBusy(err) {
			return err
		}
	}
	return err
}

// releaseClaim deletes an UNSETTLED claim, freeing the entry to be reversed again. The
// 0004 trigger permits this for an open claim only; a settled reversal is append-only and
// the DELETE is refused there.
func releaseClaim(ctx context.Context, db *sql.DB, claimID string) error {
	var err error
	for attempt := 0; attempt < 50; attempt++ {
		_, err = db.ExecContext(ctx, `DELETE FROM reversals WHERE id = ?`, claimID)
		if !isInfraBusy(err) {
			return err
		}
	}
	return err
}

// reverse is the whole operation: claim, post the compensating transfer, settle. It
// returns won=false when another caller held the claim. The idempotency key is the
// CALLER's, deliberately distinct per goroutine, so R4 cannot stand in for the 0004
// constraint.
//
// ONCE THE CLAIM COMMITS IT HOLDS THE ENTRY, so every path out of this function has to
// decide whether to keep it or release it, and the two cases are NOT symmetric:
//
//   - the compensating transfer did NOT post -> RELEASE. No money moved, so holding the
//     claim buys nothing and `UNIQUE (original_transfer_id)` would brick the entry
//     permanently. `insufficient_funds` here is the ordinary case, not an exotic one: the
//     destination has already spent what it received.
//   - the compensating transfer DID post -> KEEP, even if settling the link then fails.
//     Releasing would let the entry be reversed a second time and move money twice, which
//     is strictly worse than a link row that is correct about the block but missing its
//     pointer.
//
// This closes the caller half of the liveness defect (zola's C5). It does NOT close all of
// it: a crash between the claim's commit and the release still orphans the claim, and no
// sweep collects one. Two transactions always leave that window.
//
// Production does not have the defect at all. `core.Reverse` carries the already-reversed
// check, the R4 replay, the R7/R3 checks and the InsertTransfer inside a SINGLE
// `store.InTx`, so there is no second transaction to crash between -- verified against
// core/reversal.go, which has exactly one InTx. The window below is a property of this
// test helper, not of the system.
func reverse(ctx context.Context, db *sql.DB, l core.Ledger, t core.Transfer, claimID, idemKey string) (won bool, err error) {
	won, err = claimReversal(ctx, db, claimID, t.ID, t.Owner)
	if err != nil || !won {
		return false, err
	}
	// The compensating transfer is the original, backwards (R5: a correction is a new
	// posting, never an edit). Authorised by the account being debited, which is the
	// original's destination.
	dest, gerr := l.GetAccount(ctx, t.To)
	if gerr != nil {
		return false, releasingClaim(ctx, db, claimID, gerr)
	}
	res, terr := transferRetryResult(ctx, l, core.TransferCmd{
		Authorizer: dest.Owner, From: t.To, To: t.From,
		Amount: t.Amount, Currency: t.Currency, IdempotencyKey: idemKey,
	})
	if terr != nil {
		// Nothing posted. Hand the entry back.
		return false, releasingClaim(ctx, db, claimID, terr)
	}
	if serr := settleReversal(ctx, db, claimID, res.Transfer.ID); serr != nil {
		// The posting COMMITTED. Keep the claim: the entry really is reversed, and the
		// block it imposes is correct even though the link is incomplete.
		return true, fmt.Errorf("compensating transfer %s posted but its reversal link was "+
			"not settled (the entry stays blocked, which is correct): %w", res.Transfer.ID, serr)
	}
	return true, nil
}

// releasingClaim releases a claim whose compensating transfer never posted and returns the
// original failure. A release that itself fails is the one outcome that permanently bricks
// the entry, so it is reported loudly rather than folded into cause.
func releasingClaim(ctx context.Context, db *sql.DB, claimID string, cause error) error {
	if rerr := releaseClaim(ctx, db, claimID); rerr != nil {
		return fmt.Errorf("the compensating transfer failed (%v) AND releasing the claim "+
			"failed (%v): this entry can no longer be reversed", cause, rerr)
	}
	return cause
}

// transferRetryResult is transferRetry's sibling for callers that need the resulting
// transfer id. transferRetry discards the result; the settle step needs it.
func transferRetryResult(ctx context.Context, l core.Ledger, cmd core.TransferCmd) (core.TransferResult, error) {
	var res core.TransferResult
	var err error
	for attempt := 0; attempt < 50; attempt++ {
		res, err = l.Transfer(ctx, cmd)
		if !isInfraBusy(err) {
			return res, err
		}
	}
	return res, err
}

// --- R18: concurrent reversals of the SAME entry ---------------------------------

// TestR18_ConcurrentReversalsOfSameEntry_ExactlyOne is the double-spend property applied
// to corrections. N goroutines, released together, all try to reverse ONE transfer. The
// database must let exactly one through: one claim, one compensating transfer, and
// balances that land where a single reversal would leave them -- never doubly credited.
//
// Each goroutine carries its OWN idempotency key, so R4 cannot dedupe them. The only
// thing standing between this test and a double reversal is 0004's
// UNIQUE (original_transfer_id).
func TestR18_ConcurrentReversalsOfSameEntry_ExactlyOne(t *testing.T) {
	l, db := newLedger(t)
	ctx := context.Background()
	sys := newSystemAccount(t, l)
	a := newAccount(t, l, "alice")
	b := newAccount(t, l, "bob")

	fund(t, l, sys, a, 100)
	res, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "alice", From: a, To: b, Amount: 40, Currency: usd, IdempotencyKey: "entry",
	})
	if err != nil {
		t.Fatalf("original transfer: %v", err)
	}
	original := res.Transfer

	const N = 32
	ready, start := barrier(N)
	won := make([]bool, N)
	errs := make([]error, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-start // all N race for the one claim together
			won[i], errs[i] = reverse(ctx, db, l, original,
				fmt.Sprintf("rvsl-%d", i), // distinct claim ids: the table picks the winner
				fmt.Sprintf("rev-key-%d", i))
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()

	// Check the CLAIM COUNT before the errors, deliberately. When the guarantee is
	// removed, every goroutine wins its claim and they all race to post the same
	// compensating transfer -- so bob is overdrawn and the first thing to surface is an
	// R3 `insufficient_funds` from a transfer, which says nothing about what actually
	// broke. Verified by deleting UNIQUE (original_transfer_id) from 0004 and running
	// this test: asserting the errors first reports the symptom, asserting the claim
	// count first reports the defect.
	winners := 0
	for i := 0; i < N; i++ {
		if won[i] {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("R18: %d of %d goroutines won the reversal claim, want exactly 1. "+
			"UNIQUE (original_transfer_id) in migration 0004 is not enforcing "+
			"at-most-one-reversal, so the entry can be reversed more than once. Any "+
			"errors reported below are downstream symptoms of that, not the cause: %v",
			winners, N, errs)
	}
	for i := 0; i < N; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, errs[i])
		}
	}

	// The database agrees: one link row for this entry, and it is settled.
	if n := scalar(t, db, `SELECT COUNT(*) FROM reversals WHERE original_transfer_id = ?`, original.ID); n != 1 {
		t.Fatalf("R18: %d reversal rows for the entry, want exactly 1", n)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM reversals
		WHERE original_transfer_id = ? AND reversal_transfer_id IS NOT NULL`, original.ID); n != 1 {
		t.Fatalf("R18: the winning reversal was not settled")
	}

	// Exactly ONE compensating entry was posted. This is the assertion that fails if
	// UNIQUE (original_transfer_id) is removed: the goroutines carry distinct
	// idempotency keys, so nothing else stops a second b->a transfer.
	if n := scalar(t, db, `SELECT COUNT(*) FROM transfers WHERE from_account = ? AND to_account = ?`,
		string(b), string(a)); n != 1 {
		t.Fatalf("R18: %d compensating transfers posted, want exactly 1 (a double reversal)", n)
	}

	// Balances are where a single reversal leaves them: the 40 went out and came back.
	if got := balance(t, l, b); got != 0 {
		t.Fatalf("R18: bob %d after one reversal, want 0", got)
	}
	if got := balance(t, l, a); got != 100 {
		t.Fatalf("R18: alice %d after one reversal, want 100", got)
	}
	if failed := assertInvariants(t, db, l, a, b, sys); failed != 0 {
		t.Fatalf("R18: %d invariant violation(s) under contention", failed)
	}
}

// --- R17: concurrent reversals of DIFFERENT entries ------------------------------

// TestR17_ConcurrentReversalsOfDifferentEntries_AllLand is the other half, and the one
// that keeps the constraint honest. A uniqueness guarantee is trivially satisfiable by
// refusing everything; this asserts the opposite edge -- M distinct entries reversed
// concurrently ALL succeed. Nothing is lost (R17), and the constraint does not serialize
// independent work into uselessness.
func TestR17_ConcurrentReversalsOfDifferentEntries_AllLand(t *testing.T) {
	l, db := newLedger(t)
	ctx := context.Background()
	sys := newSystemAccount(t, l)
	a := newAccount(t, l, "alice")
	b := newAccount(t, l, "bob")

	const M = 24
	fund(t, l, sys, a, M)

	originals := make([]core.Transfer, M)
	for i := 0; i < M; i++ {
		res, err := l.Transfer(ctx, core.TransferCmd{
			Authorizer: "alice", From: a, To: b, Amount: 1, Currency: usd,
			IdempotencyKey: fmt.Sprintf("entry-%d", i),
		})
		if err != nil {
			t.Fatalf("original transfer %d: %v", i, err)
		}
		originals[i] = res.Transfer
	}
	if got := balance(t, l, b); got != M {
		t.Fatalf("setup: bob %d, want %d", got, M)
	}

	ready, start := barrier(M)
	won := make([]bool, M)
	errs := make([]error, M)
	var wg sync.WaitGroup
	wg.Add(M)
	for i := 0; i < M; i++ {
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-start // M independent reversals, all contending for the one write lock
			won[i], errs[i] = reverse(ctx, db, l, originals[i],
				fmt.Sprintf("rvsl-%d", i), fmt.Sprintf("rev-key-%d", i))
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()

	for i := 0; i < M; i++ {
		if errs[i] != nil {
			t.Fatalf("R17: reversal %d errored: %v", i, errs[i])
		}
		if !won[i] {
			t.Fatalf("R17: reversal %d of a DISTINCT entry was refused -- the constraint "+
				"is serializing independent reversals into uselessness", i)
		}
	}

	if n := scalar(t, db, `SELECT COUNT(*) FROM reversals WHERE reversal_transfer_id IS NOT NULL`); n != M {
		t.Fatalf("R17: %d settled reversals, want %d", n, M)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM transfers WHERE from_account = ? AND to_account = ?`,
		string(b), string(a)); n != M {
		t.Fatalf("R17: %d compensating transfers, want %d", n, M)
	}
	// Every reversal links a DISTINCT original to a DISTINCT compensating transfer.
	if n := scalar(t, db, `SELECT COUNT(DISTINCT original_transfer_id) FROM reversals`); n != M {
		t.Fatalf("R17: %d distinct originals linked, want %d", n, M)
	}
	if n := scalar(t, db, `SELECT COUNT(DISTINCT reversal_transfer_id) FROM reversals
		WHERE reversal_transfer_id IS NOT NULL`); n != M {
		t.Fatalf("R17: %d distinct compensating transfers linked, want %d", n, M)
	}

	// Nothing lost: every delta applied and then came back.
	if got := balance(t, l, b); got != 0 {
		t.Fatalf("R17: bob %d after %d reversals, want 0", got, M)
	}
	if got := balance(t, l, a); got != M {
		t.Fatalf("R17: alice %d after %d reversals, want %d", got, M, M)
	}
	if failed := assertInvariants(t, db, l, a, b, sys); failed != 0 {
		t.Fatalf("R17: %d invariant violation(s) under contention", failed)
	}
}

// --- the constraints in isolation ------------------------------------------------

// TestReversalConstraints_AreEnforcedByTheDatabase exercises each guarantee 0004 carries
// directly against SQL, with no concurrency in the way. The concurrency tests above prove
// the guarantees hold under contention; this one proves they are the DATABASE's and
// names each refusal, so a weakened migration fails here with a specific message rather
// than as a mysterious race.
func TestReversalConstraints_AreEnforcedByTheDatabase(t *testing.T) {
	l, db := newLedger(t)
	ctx := context.Background()
	sys := newSystemAccount(t, l)
	a := newAccount(t, l, "alice")
	b := newAccount(t, l, "bob")
	fund(t, l, sys, a, 100)

	mk := func(key string, amount core.Amount) core.Transfer {
		t.Helper()
		res, err := l.Transfer(ctx, core.TransferCmd{
			Authorizer: "alice", From: a, To: b, Amount: amount, Currency: usd, IdempotencyKey: key,
		})
		if err != nil {
			t.Fatalf("transfer %s: %v", key, err)
		}
		return res.Transfer
	}
	t1, t2 := mk("t1", 10), mk("t2", 10)
	comp := mk("comp", 1) // stands in as a compensating transfer id

	now := time.Now().UTC().Format(time.RFC3339Nano)
	claim := func(id, original string) error {
		_, err := db.ExecContext(ctx,
			`INSERT INTO reversals(id, original_transfer_id, owner, created_at) VALUES(?,?,?,?)`,
			id, original, "alice", now)
		return err
	}

	// An entry is reversible at most once.
	if err := claim("r1", t1.ID); err != nil {
		t.Fatalf("first claim should succeed: %v", err)
	}
	if err := claim("r1-again", t1.ID); err == nil {
		t.Fatal("a SECOND claim on the same entry must be refused by UNIQUE (original_transfer_id)")
	} else if !isUniqueViolation(err) {
		t.Fatalf("second claim refused, but not by a UNIQUE constraint: %v", err)
	}

	// A different entry is unaffected.
	if err := claim("r2", t2.ID); err != nil {
		t.Fatalf("claiming a DIFFERENT entry must succeed: %v", err)
	}

	// The settle columns move together: half-settled is not representable.
	if _, err := db.ExecContext(ctx,
		`UPDATE reversals SET reversal_transfer_id = ? WHERE id = 'r1'`, comp.ID); err == nil {
		t.Fatal("setting reversal_transfer_id without settled_at must fail the CHECK")
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE reversals SET settled_at = ? WHERE id = 'r1'`, now); err == nil {
		t.Fatal("setting settled_at without reversal_transfer_id must fail the CHECK")
	}

	// A proper settle succeeds.
	if err := settleReversal(ctx, db, "r1", comp.ID); err != nil {
		t.Fatalf("settle should succeed: %v", err)
	}

	// One compensating transfer settles at most one reversal.
	if err := settleReversal(ctx, db, "r2", comp.ID); err == nil {
		t.Fatal("two reversals settling to the SAME compensating transfer must be refused")
	}

	// Settle is one-way: no repointing, no unsettling.
	if _, err := db.ExecContext(ctx,
		`UPDATE reversals SET reversal_transfer_id = ?, settled_at = ? WHERE id = 'r1'`,
		t2.ID, now); err == nil {
		t.Fatal("repointing a settled reversal must be refused")
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE reversals SET reversal_transfer_id = NULL, settled_at = NULL WHERE id = 'r1'`); err == nil {
		t.Fatal("unsettling a reversal must be refused -- it would free the entry to be reversed twice")
	}

	// The original is immutable once claimed.
	if _, err := db.ExecContext(ctx,
		`UPDATE reversals SET original_transfer_id = ? WHERE id = 'r1'`, t2.ID); err == nil {
		t.Fatal("repointing original_transfer_id must be refused")
	}

	// A settled reversal is append-only; an OPEN claim is deletable, which is the
	// rollback path when the compensating transfer fails after the claim commits.
	if _, err := db.ExecContext(ctx, `DELETE FROM reversals WHERE id = 'r1'`); err == nil {
		t.Fatal("deleting a SETTLED reversal must be refused")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM reversals WHERE id = 'r2'`); err != nil {
		t.Fatalf("deleting an OPEN claim is the rollback path and must succeed: %v", err)
	}
	// Released, the entry is claimable again.
	if err := claim("r2-retry", t2.ID); err != nil {
		t.Fatalf("after releasing an open claim the entry must be claimable again: %v", err)
	}

	// A transfer cannot reverse itself.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO reversals(id, original_transfer_id, reversal_transfer_id, owner, created_at, settled_at)
		 VALUES('self', ?, ?, 'alice', ?, ?)`, comp.ID, comp.ID, now, now); err == nil {
		t.Fatal("a transfer reversing itself must be refused by the CHECK")
	}

	// The link is referential: it cannot name a transfer that does not exist.
	if err := claim("ghost", "no-such-transfer"); err == nil {
		t.Fatal("a reversal naming an unknown transfer must be refused by the foreign key")
	}
}

// --- C5: a failed compensating transfer must not brick the entry ------------------

// TestC5_FailedCompensatingTransfer_LeavesTheEntryReversible is the liveness property the
// no-double-reversal guarantee can destroy if the caller is careless. Claim-then-settle
// spans two transactions, so a claim can commit and the compensating transfer can then
// fail -- and UNIQUE (original_transfer_id) would make that entry permanently
// unreversible, which is a money-path defect, not an inconvenience.
//
// The failure exercised here is the ordinary one named in review: the destination has
// already SPENT what it received, so the compensating debit hits R3 insufficient_funds.
// The entry must survive that and still be reversible once the funds are back.
func TestC5_FailedCompensatingTransfer_LeavesTheEntryReversible(t *testing.T) {
	l, db := newLedger(t)
	ctx := context.Background()
	sys := newSystemAccount(t, l)
	a := newAccount(t, l, "alice")
	b := newAccount(t, l, "bob")
	c := newAccount(t, l, "carol")

	fund(t, l, sys, a, 100)
	res, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "alice", From: a, To: b, Amount: 40, Currency: usd, IdempotencyKey: "entry",
	})
	if err != nil {
		t.Fatalf("original transfer: %v", err)
	}
	original := res.Transfer

	// Bob spends every unit he received, so the compensating debit cannot be afforded.
	if _, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "bob", From: b, To: c, Amount: 40, Currency: usd, IdempotencyKey: "spent",
	}); err != nil {
		t.Fatalf("bob spends the funds: %v", err)
	}
	if got := balance(t, l, b); got != 0 {
		t.Fatalf("setup: bob %d, want 0", got)
	}

	// The reversal must FAIL -- and must hand the entry back.
	won, err := reverse(ctx, db, l, original, "rvsl-doomed", "rev-doomed")
	if err == nil {
		t.Fatal("C5: reversing into an emptied account should fail on the compensating debit")
	}
	if !errors.Is(err, core.ErrInsufficientFunds) {
		t.Fatalf("C5: want the R3 rejection, got %v", err)
	}
	if won {
		t.Fatal("C5: reverse reported it still holds the claim after the posting failed")
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM reversals WHERE original_transfer_id = ?`,
		original.ID); n != 0 {
		t.Fatalf("C5: %d claim row(s) stranded after a failed compensating transfer. "+
			"UNIQUE (original_transfer_id) now blocks this entry forever -- the money-path "+
			"liveness defect", n)
	}
	// Nothing moved on the failed attempt.
	if got := balance(t, l, a); got != 60 {
		t.Fatalf("C5: alice %d after the FAILED reversal, want 60 (nothing should have moved)", got)
	}

	// Funds come back, and the entry is still reversible -- the whole point.
	if _, err := l.Transfer(ctx, core.TransferCmd{
		Authorizer: "carol", From: c, To: b, Amount: 40, Currency: usd, IdempotencyKey: "refund",
	}); err != nil {
		t.Fatalf("carol refunds bob: %v", err)
	}
	won, err = reverse(ctx, db, l, original, "rvsl-retry", "rev-retry")
	if err != nil {
		t.Fatalf("C5: the entry must still be reversible after a failed attempt, got %v", err)
	}
	if !won {
		t.Fatal("C5: the retry did not win the claim -- the released claim was not released")
	}

	// And it settled properly: one link row, pointing at a real compensating transfer.
	if n := scalar(t, db, `SELECT COUNT(*) FROM reversals
		WHERE original_transfer_id = ? AND reversal_transfer_id IS NOT NULL`, original.ID); n != 1 {
		t.Fatalf("C5: want exactly 1 settled reversal after the successful retry, got %d", n)
	}
	if got := balance(t, l, a); got != 100 {
		t.Fatalf("C5: alice %d after the successful reversal, want 100", got)
	}

	// The entry is now genuinely spent: a third attempt is refused, as it must be.
	if won, _ := reverse(ctx, db, l, original, "rvsl-third", "rev-third"); won {
		t.Fatal("C5: an already-reversed entry must not be reversible again")
	}
	if failed := assertInvariants(t, db, l, a, b, c, sys); failed != 0 {
		t.Fatalf("C5: %d invariant violation(s)", failed)
	}
}

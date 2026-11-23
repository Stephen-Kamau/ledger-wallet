package core

import (
	"context"
	"strings"
)

// reversalPrefix tags the DERIVED id of a compensating entry. The reversal of
// transfer X is always written with id reversalPrefix+X, which makes "has this
// entry already been reversed?" a primary-key lookup rather than a scan, and makes
// "at most one reversal per entry" an invariant the STORAGE enforces: the second
// insert collides on the transfers primary key even if two writers somehow raced
// past the in-transaction check.
//
// Deriving the id is what lets reversal ship without a schema change. The obvious
// alternative - a nullable reversal_of column - would have to be added to BOTH the
// adapter's embeddedSchema and migrations/, and those two drifting apart is exactly
// the defect collision-v2 shipped (the currencies table lived only in migrations/,
// so the server booted without it while every unit test stayed green). No column,
// no second schema source, no drift.
const reversalPrefix = "rev_"

// reversalID is the derived id of the compensating entry for entryID.
func reversalID(entryID string) string { return reversalPrefix + entryID }

// isReversal reports whether id is itself a compensating entry. Reversing a
// reversal is allowed - it is a distinct entry and a legitimate correction of a
// correction - so this is not a rejection, only a description.
func isReversal(id string) bool { return strings.HasPrefix(id, reversalPrefix) }

// Reverse posts a compensating entry that mirrors entryID's postings (R5).
//
// The ordering mirrors Transfer (00-core.md §3): cheap, caller-visible rejections
// happen before the transaction opens, and everything that depends on live state -
// the R4 replay, the already-reversed check, and the R3 sufficiency check - happens
// inside it under the single serialized writer (§5). The original is only ever READ;
// the correction is only ever an INSERT.
func (l *ledger) Reverse(ctx context.Context, authorizer PrincipalID, entryID string, idempotencyKey string) (TransferResult, error) {
	if entryID == "" {
		return TransferResult{}, ErrTransferNotFound
	}

	var result TransferResult
	err := l.store.InTx(ctx, func(tx Tx) error {
		// R4: a replayed key returns the original reversal and posts nothing. This
		// runs first so a retry is cheap and can never double-post.
		prior, ok, err := tx.FindTransferByKey(authorizer, idempotencyKey)
		if err != nil {
			return err
		}
		if ok {
			if prior.ID != reversalID(entryID) {
				// The same key was already used for a different entry (R4).
				return ErrIdempotencyConflict
			}
			result = TransferResult{Transfer: prior, Replayed: true}
			return nil
		}

		original, ok, err := tx.FindTransferByID(entryID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrTransferNotFound
		}

		// R13: one entry, at most one compensating entry.
		if _, reversed, err := tx.FindTransferByID(reversalID(entryID)); err != nil {
			return err
		} else if reversed {
			return ErrAlreadyReversed
		}

		// The compensation debits the original's destination, so that account's
		// owner is the one who must authorise it (R7) and fund it (R3).
		debited, err := tx.GetAccount(original.To)
		if err != nil {
			return err
		}
		if debited.Owner != authorizer {
			return ErrUnauthorized
		}

		entries := mirror(original.Postings)
		if err := assertBalanced(entries); err != nil { // R1
			return err
		}

		if !debited.AllowNegative { // R3, on the live balance under the write lock
			bal, berr := tx.Balance(original.To)
			if berr != nil {
				return berr
			}
			if bal+debitLeg(entries) < 0 {
				return ErrInsufficientFunds
			}
		}

		rev := l.materialiseReversal(original, authorizer, idempotencyKey, entries)
		if ierr := tx.InsertTransfer(rev); ierr != nil {
			return ierr
		}
		result = TransferResult{Transfer: rev, Replayed: false}
		return nil
	})
	if err != nil {
		return TransferResult{}, err
	}
	return result, nil
}

// mirror turns the original postings into the compensating legs: the same accounts,
// each amount negated. Mirroring the POSTINGS rather than rebuilding from the
// transfer's from/to/amount means the compensation is an exact inverse of what was
// actually written, whatever shape it had.
func mirror(postings []Posting) []Entry {
	entries := make([]Entry, 0, len(postings))
	for _, p := range postings {
		entries = append(entries, Entry{Account: p.Account, Amount: -p.Amount})
	}
	return entries
}

// assertBalanced enforces R1 on a set of entries. The mirror of a balanced set is
// balanced, so this can only fire if the original was written unbalanced - in which
// case refusing to compensate it is the right answer, not posting a second
// unbalanced entry on top.
func assertBalanced(entries []Entry) error {
	if len(entries) == 0 {
		return ErrUnbalanced
	}
	var sum Amount
	for _, e := range entries {
		sum += e.Amount
	}
	if sum != 0 {
		return ErrUnbalanced
	}
	return nil
}

// materialiseReversal builds the compensating Transfer. Its id is derived from the
// original (see reversalPrefix); its direction is the original's reversed, so the
// record reads naturally as "To paid From back". Posting.Seq is left zero: the
// adapter assigns it under the write lock (D-B1).
func (l *ledger) materialiseReversal(original Transfer, authorizer PrincipalID, key string, entries []Entry) Transfer {
	now := l.now()
	tid := reversalID(original.ID)
	postings := make([]Posting, 0, len(entries))
	for _, e := range entries {
		postings = append(postings, Posting{
			ID:         newID("post"),
			TransferID: tid,
			Account:    e.Account,
			Amount:     e.Amount,
			Currency:   original.Currency,
			CreatedAt:  now,
		})
	}
	return Transfer{
		ID:             tid,
		IdempotencyKey: key,
		Owner:          authorizer,
		From:           original.To,
		To:             original.From,
		Amount:         original.Amount,
		Currency:       original.Currency,
		CreatedAt:      now,
		Postings:       postings,
	}
}

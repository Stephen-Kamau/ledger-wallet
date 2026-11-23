package core

import "context"

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

// reversalID is the derived id of the compensating entry for entryID. Applying it
// to a reversal's own id is how reversing a reversal works: that is a DISTINCT
// entry and a legitimate correction of a correction, so it is allowed, while a
// second reversal of the SAME entry collides and is refused.
func reversalID(entryID string) string { return reversalPrefix + entryID }

// Reverse posts a compensating entry that mirrors entryID's postings (R5).
//
// The ordering mirrors Transfer (00-core.md §3): cheap, caller-visible rejections
// happen before the transaction opens, and everything that depends on live state -
// the R4 replay, the already-reversed check, and the R3 sufficiency check - happens
// inside it under the single serialized writer (§5). The original is only ever READ;
// the correction is only ever an INSERT.
func (l *ledger) Reverse(ctx context.Context, authorizer PrincipalID, entryID string, idempotencyKey string) (TransferResult, error) {
	if entryID == "" {
		return TransferResult{}, ErrEntryNotFound
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
			return ErrEntryNotFound
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

		// R3, on live balances under the held write lock. Checked PER DEBITED
		// ACCOUNT rather than by applying the whole debit total to original.To:
		// mirror() is written over arbitrary postings, so for an entry with more
		// than two legs a single combined check would charge one account for every
		// other account's debit. Today every entry has exactly two legs and the two
		// forms agree; this one keeps agreeing if that changes.
		for account, delta := range netByAccount(entries) {
			if delta >= 0 {
				continue // a credit never needs funding
			}
			acct, aerr := tx.GetAccount(account)
			if aerr != nil {
				return aerr
			}
			if acct.AllowNegative {
				continue
			}
			bal, berr := tx.Balance(account)
			if berr != nil {
				return berr
			}
			if bal+delta < 0 {
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

// netByAccount collapses entries to one signed total per account, so a
// sufficiency check looks at what each account is actually losing.
func netByAccount(entries []Entry) map[AccountID]Amount {
	net := make(map[AccountID]Amount, len(entries))
	for _, e := range entries {
		net[e.Account] += e.Amount
	}
	return net
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

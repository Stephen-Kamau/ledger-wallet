package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// txn is the transactional surface (core.Tx) over a *sql.Tx opened with BEGIN
// IMMEDIATE. All reads it makes are held under the write lock (§5). It exposes
// INSERTs and reads but no UPDATE/DELETE of postings - append-only (R5) is
// enforced by the absence of a method here and by the storage triggers.
type txn struct {
	tx  *sql.Tx
	ctx context.Context
}

// GetAccount reads an account under the held write lock.
func (t *txn) GetAccount(id core.AccountID) (core.Account, error) {
	return scanAccount(t.tx.QueryRowContext(t.ctx, selectAccount, string(id)))
}

// Balance returns the live derived balance under the held write lock - this is the
// read the R3 sufficiency check depends on being atomic with the write (§5).
func (t *txn) Balance(id core.AccountID) (core.Amount, error) {
	return balance(t.ctx, t.tx, id)
}

// InsertAccount writes a new account row (R6).
func (t *txn) InsertAccount(a core.Account) error {
	allowNeg := 0
	if a.AllowNegative {
		allowNeg = 1
	}
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO accounts (id, owner, currency, allow_negative, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		string(a.ID), string(a.Owner), string(a.Currency), allowNeg,
		a.CreatedAt.UTC().Format(rfc3339))
	if err != nil {
		return fmt.Errorf("insert account: %w", err)
	}
	return nil
}

// FindTransferByKey is the R4 fast-path lookup, scoped per owner. It loads the
// transfer and its postings so a replay returns the full original result.
func (t *txn) FindTransferByKey(owner core.PrincipalID, key string) (core.Transfer, bool, error) {
	row := t.tx.QueryRowContext(t.ctx,
		`SELECT id, owner, idempotency_key, from_account, to_account, amount, currency, created_at
		 FROM transfers WHERE owner = ? AND idempotency_key = ?`,
		string(owner), key)

	tr, err := scanTransfer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Transfer{}, false, nil
	}
	if err != nil {
		return core.Transfer{}, false, err
	}
	postings, perr := t.postingsOf(tr.ID)
	if perr != nil {
		return core.Transfer{}, false, perr
	}
	tr.Postings = postings
	return tr, true, nil
}

// InsertTransfer writes the transfer row and all its postings atomically, assigning
// each posting's seq as MAX(seq)+1 for its account under the held write lock (D-B1).
// The engine never inserts a posting without its transfer.
func (t *txn) InsertTransfer(tr core.Transfer) error {
	if _, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO transfers (id, owner, idempotency_key, from_account, to_account, amount, currency, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tr.ID, string(tr.Owner), tr.IdempotencyKey, string(tr.From), string(tr.To),
		int64(tr.Amount), string(tr.Currency), tr.CreatedAt.UTC().Format(rfc3339),
	); err != nil {
		return fmt.Errorf("insert transfer: %w", err)
	}

	// Index by reference so the assigned seq is written back into the caller's
	// Transfer (D-B1): the adapter is the authority for seq, and the returned result
	// -> the 201 body (10-api.md §1.3) must carry the real value, not the zero the
	// engine left. Previously this ranged by value and dropped the write-back, so a
	// first-write result reported seq:0 while the DB held the true monotonic seq.
	for i := range tr.Postings {
		p := &tr.Postings[i]
		seq, err := t.nextSeq(p.Account)
		if err != nil {
			return err
		}
		p.Seq = seq // authoritative, assigned under the write lock
		if _, err := t.tx.ExecContext(t.ctx,
			`INSERT INTO postings (id, transfer_id, account_id, amount, currency, seq, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.TransferID, string(p.Account), int64(p.Amount), string(p.Currency),
			seq, p.CreatedAt.UTC().Format(rfc3339),
		); err != nil {
			return fmt.Errorf("insert posting: %w", err)
		}
	}
	return nil
}

// nextSeq computes MAX(seq)+1 for an account under the write lock (D-B1). Because
// the whole closure holds the BEGIN IMMEDIATE write lock, no concurrent writer can
// interleave, so the computed seq is collision-free; the UNIQUE(account_id, seq)
// index is the loud backstop if that ever fails.
func (t *txn) nextSeq(account core.AccountID) (int64, error) {
	var next int64
	if err := t.tx.QueryRowContext(t.ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM postings WHERE account_id = ?`,
		string(account)).Scan(&next); err != nil {
		return 0, fmt.Errorf("next seq: %w", err)
	}
	return next, nil
}

func (t *txn) postingsOf(transferID string) ([]core.Posting, error) {
	rows, err := t.tx.QueryContext(t.ctx,
		`SELECT id, transfer_id, account_id, amount, currency, seq, created_at
		 FROM postings WHERE transfer_id = ? ORDER BY seq ASC`, transferID)
	if err != nil {
		return nil, fmt.Errorf("postings of transfer: %w", err)
	}
	defer rows.Close()

	var out []core.Posting
	for rows.Next() {
		p, serr := scanPosting(rows)
		if serr != nil {
			return nil, serr
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanTransfer(row rowScanner) (core.Transfer, error) {
	var (
		tr                                      core.Transfer
		id, owner, key, from, to, curr, created string
		amount                                  int64
	)
	if err := row.Scan(&id, &owner, &key, &from, &to, &amount, &curr, &created); err != nil {
		return core.Transfer{}, err
	}
	ts, err := time.Parse(rfc3339, created)
	if err != nil {
		return core.Transfer{}, fmt.Errorf("parse created_at: %w", err)
	}
	tr.ID = id
	tr.Owner = core.PrincipalID(owner)
	tr.IdempotencyKey = key
	tr.From = core.AccountID(from)
	tr.To = core.AccountID(to)
	tr.Amount = core.Amount(amount)
	tr.Currency = core.Currency(curr)
	tr.CreatedAt = ts
	return tr, nil
}

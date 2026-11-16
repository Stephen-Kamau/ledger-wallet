package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// This file is the hold surface of the adapter (account holds): a reservation of
// funds that posts nothing. All write paths run under the BEGIN IMMEDIATE write lock
// (§5) via core.Store.InTx, so the available-funds check and the insert are atomic
// with respect to every other writer, exactly as Transfer's R3 check is.

const (
	sumActiveHolds = `SELECT COALESCE(SUM(amount), 0) FROM holds
	                  WHERE account_id = ? AND active = 1`
	selectHoldByKey = `SELECT id, account_id, owner, idempotency_key, amount, currency, active, created_at, released_at
	                   FROM holds WHERE owner = ? AND idempotency_key = ?`
	selectHold = `SELECT id, account_id, owner, idempotency_key, amount, currency, active, created_at, released_at
	              FROM holds WHERE id = ?`
)

// ActiveHoldsTotal (read-only path) sums an account's active holds outside a write
// transaction. It guards unknown accounts the same way Balance does.
func (s *Store) ActiveHoldsTotal(ctx context.Context, id core.AccountID) (core.Amount, error) {
	if _, err := s.GetAccount(ctx, id); err != nil {
		return 0, err
	}
	return activeHoldsTotal(ctx, s.db, id)
}

// ActiveHoldsTotal returns the live reserved sum under the held write lock.
func (t *txn) ActiveHoldsTotal(id core.AccountID) (core.Amount, error) {
	return activeHoldsTotal(t.ctx, t.tx, id)
}

func activeHoldsTotal(ctx context.Context, q querier, id core.AccountID) (core.Amount, error) {
	var sum int64
	if err := q.QueryRowContext(ctx, sumActiveHolds, string(id)).Scan(&sum); err != nil {
		return 0, fmt.Errorf("active holds sum: %w", err)
	}
	return core.Amount(sum), nil
}

// FindHoldByKey is the idempotency lookup, scoped per owner (mirrors
// FindTransferByKey). A replay returns the original hold.
func (t *txn) FindHoldByKey(owner core.PrincipalID, key string) (core.Hold, bool, error) {
	h, err := scanHold(t.tx.QueryRowContext(t.ctx, selectHoldByKey, string(owner), key))
	if errors.Is(err, sql.ErrNoRows) {
		return core.Hold{}, false, nil
	}
	if err != nil {
		return core.Hold{}, false, err
	}
	return h, true, nil
}

// GetHold loads a hold by id for a release. The bool is false for an unknown id.
func (t *txn) GetHold(id core.HoldID) (core.Hold, bool, error) {
	h, err := scanHold(t.tx.QueryRowContext(t.ctx, selectHold, string(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return core.Hold{}, false, nil
	}
	if err != nil {
		return core.Hold{}, false, err
	}
	return h, true, nil
}

// InsertHold writes an active hold row. The UNIQUE(owner, idempotency_key) index is
// the backstop under contention, as it is for transfers.
func (t *txn) InsertHold(h core.Hold) error {
	active := 0
	if h.Active {
		active = 1
	}
	var released any // NULL while active
	if !h.ReleasedAt.IsZero() {
		released = h.ReleasedAt.UTC().Format(rfc3339)
	}
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO holds (id, account_id, owner, idempotency_key, amount, currency, active, created_at, released_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(h.ID), string(h.Account), string(h.Owner), h.IdempotencyKey,
		int64(h.Amount), string(h.Currency), active, h.CreatedAt.UTC().Format(rfc3339), released)
	if err != nil {
		return fmt.Errorf("insert hold: %w", err)
	}
	return nil
}

// ReleaseHold marks an active hold inactive and stamps released_at under the write
// lock. It scopes the UPDATE to active = 1 so a concurrent double-release writes
// nothing; the engine's GetHold check already rejected an inactive hold.
func (t *txn) ReleaseHold(id core.HoldID) error {
	_, err := t.tx.ExecContext(t.ctx,
		`UPDATE holds SET active = 0, released_at = ? WHERE id = ? AND active = 1`,
		time.Now().UTC().Format(rfc3339), string(id))
	if err != nil {
		return fmt.Errorf("release hold: %w", err)
	}
	return nil
}

func scanHold(row rowScanner) (core.Hold, error) {
	var (
		h                                     core.Hold
		id, account, owner, key, currency, cr string
		amount, active                        int64
		released                              sql.NullString
	)
	if err := row.Scan(&id, &account, &owner, &key, &amount, &currency, &active, &cr, &released); err != nil {
		return core.Hold{}, err
	}
	created, err := time.Parse(rfc3339, cr)
	if err != nil {
		return core.Hold{}, fmt.Errorf("parse created_at: %w", err)
	}
	h.ID = core.HoldID(id)
	h.Account = core.AccountID(account)
	h.Owner = core.PrincipalID(owner)
	h.IdempotencyKey = key
	h.Amount = core.Amount(amount)
	h.Currency = core.Currency(currency)
	h.Active = active != 0
	h.CreatedAt = created
	if released.Valid {
		ts, perr := time.Parse(rfc3339, released.String)
		if perr != nil {
			return core.Hold{}, fmt.Errorf("parse released_at: %w", perr)
		}
		h.ReleasedAt = ts
	}
	return h, nil
}

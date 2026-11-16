package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

const rfc3339 = time.RFC3339Nano

// InTx runs fn inside a single BEGIN IMMEDIATE write transaction (via the DSN's
// _txlock=immediate), committing on success and rolling back on any error. This is
// the serialization primitive of §5: the write lock is held for the whole closure,
// so FindTransferByKey + Balance + InsertTransfer are atomic with respect to every
// other writer.
func (s *Store) InTx(ctx context.Context, fn func(tx core.Tx) error) error {
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin immediate: %w", err)
	}
	if err := fn(&txn{tx: sqlTx, ctx: ctx}); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// GetAccount reads an account outside a write transaction (read-only path).
func (s *Store) GetAccount(ctx context.Context, id core.AccountID) (core.Account, error) {
	return scanAccount(s.db.QueryRowContext(ctx, selectAccount, string(id)))
}

// AccountsByOwner returns every account owned by a principal (R20, D-C1), ordered
// by creation for a stable view.
func (s *Store) AccountsByOwner(ctx context.Context, owner core.PrincipalID) ([]core.Account, error) {
	rows, err := s.db.QueryContext(ctx, selectAccountsByOwner, string(owner))
	if err != nil {
		return nil, fmt.Errorf("accounts by owner: %w", err)
	}
	defer rows.Close()

	var out []core.Account
	for rows.Next() {
		a, serr := scanAccountRows(rows)
		if serr != nil {
			return nil, serr
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Balance returns the derived balance (sum of postings) for an account (R10). It
// returns ErrAccountNotFound for an unknown account so a read of a missing account
// is distinguishable from a real zero balance.
func (s *Store) Balance(ctx context.Context, id core.AccountID) (core.Amount, error) {
	if _, err := s.GetAccount(ctx, id); err != nil {
		return 0, err
	}
	return balance(ctx, s.db, id)
}

// Statement returns the account's postings keyset-paged on seq (R12).
func (s *Store) Statement(ctx context.Context, id core.AccountID, page core.Page) (core.StatementPage, error) {
	if _, err := s.GetAccount(ctx, id); err != nil {
		return core.StatementPage{}, err
	}
	limit := page.Limit
	if limit <= 0 {
		limit = 50
	}
	// Fetch one extra to detect HasMore without a second query.
	rows, err := s.db.QueryContext(ctx, selectStatement, string(id), page.AfterSeq, limit+1)
	if err != nil {
		return core.StatementPage{}, fmt.Errorf("statement: %w", err)
	}
	defer rows.Close()

	var postings []core.Posting
	for rows.Next() {
		p, serr := scanPosting(rows)
		if serr != nil {
			return core.StatementPage{}, serr
		}
		postings = append(postings, p)
	}
	if err := rows.Err(); err != nil {
		return core.StatementPage{}, err
	}

	out := core.StatementPage{}
	if len(postings) > limit {
		out.HasMore = true
		postings = postings[:limit]
	}
	out.Postings = postings
	if n := len(postings); n > 0 {
		out.NextAfterSeq = postings[n-1].Seq
	} else {
		out.NextAfterSeq = page.AfterSeq
	}
	return out, nil
}

// ListCurrencies returns the currency reference rows (migration 0002), ordered by
// code. Read-only path; the table is seeded by the migrator, not this adapter's
// embeddedSchema, so this reads whatever the migrated DB holds.
func (s *Store) ListCurrencies(ctx context.Context) ([]core.CurrencyMeta, error) {
	rows, err := s.db.QueryContext(ctx, selectCurrencies)
	if err != nil {
		return nil, fmt.Errorf("list currencies: %w", err)
	}
	defer rows.Close()

	var out []core.CurrencyMeta
	for rows.Next() {
		var c core.CurrencyMeta
		if err := rows.Scan(&c.Code, &c.MinorUnit, &c.Symbol); err != nil {
			return nil, fmt.Errorf("scan currency: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- shared query helpers -------------------------------------------------------

const (
	selectAccount = `SELECT id, owner, currency, allow_negative, created_at
	                 FROM accounts WHERE id = ?`
	selectAccountsByOwner = `SELECT id, owner, currency, allow_negative, created_at
	                         FROM accounts WHERE owner = ? ORDER BY created_at, id`
	sumPostings     = `SELECT COALESCE(SUM(amount), 0) FROM postings WHERE account_id = ?`
	selectStatement = `SELECT id, transfer_id, account_id, amount, currency, seq, created_at
	                   FROM postings WHERE account_id = ? AND seq > ?
	                   ORDER BY seq ASC LIMIT ?`
	selectCurrencies = `SELECT code, minor_unit, symbol FROM currencies ORDER BY code`
)

// rowScanner abstracts *sql.Row and *sql.Rows for the account scanners.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanAccount(row rowScanner) (core.Account, error) {
	a, err := scanAccountRows(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Account{}, core.ErrAccountNotFound
	}
	return a, err
}

func scanAccountRows(row rowScanner) (core.Account, error) {
	var (
		a            core.Account
		id, owner    string
		currency     string
		allowNeg     int64
		createdAtStr string
	)
	if err := row.Scan(&id, &owner, &currency, &allowNeg, &createdAtStr); err != nil {
		return core.Account{}, err
	}
	ts, err := time.Parse(rfc3339, createdAtStr)
	if err != nil {
		return core.Account{}, fmt.Errorf("parse created_at: %w", err)
	}
	a.ID = core.AccountID(id)
	a.Owner = core.PrincipalID(owner)
	a.Currency = core.Currency(currency)
	a.AllowNegative = allowNeg != 0
	a.CreatedAt = ts
	return a, nil
}

func scanPosting(row rowScanner) (core.Posting, error) {
	var (
		p                       core.Posting
		id, transferID, account string
		amount, seq             int64
		currency, createdAtStr  string
	)
	if err := row.Scan(&id, &transferID, &account, &amount, &currency, &seq, &createdAtStr); err != nil {
		return core.Posting{}, err
	}
	ts, err := time.Parse(rfc3339, createdAtStr)
	if err != nil {
		return core.Posting{}, fmt.Errorf("parse created_at: %w", err)
	}
	p.ID = id
	p.TransferID = transferID
	p.Account = core.AccountID(account)
	p.Amount = core.Amount(amount)
	p.Currency = core.Currency(currency)
	p.Seq = seq
	p.CreatedAt = ts
	return p, nil
}

// querier is satisfied by both *sql.DB and *sql.Tx so balance() serves the
// read-only path and the in-transaction live read with one implementation.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func balance(ctx context.Context, q querier, id core.AccountID) (core.Amount, error) {
	var sum int64
	if err := q.QueryRowContext(ctx, sumPostings, string(id)).Scan(&sum); err != nil {
		return 0, fmt.Errorf("balance sum: %w", err)
	}
	return core.Amount(sum), nil
}

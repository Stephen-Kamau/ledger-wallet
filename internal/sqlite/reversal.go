package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// FindTransferByID loads a transfer and its postings by primary key, under the
// held write lock. Reversal uses it twice: once for the entry being compensated,
// and once for that entry's DERIVED reversal id, which is how "already reversed"
// is detected without a reversal_of column (see core/reversal.go).
//
// It is a read. The append-only guarantee (R5) is untouched: there is still no
// UPDATE or DELETE of postings anywhere on this type, and the storage triggers
// remain the backstop.
func (t *txn) FindTransferByID(id string) (core.Transfer, bool, error) {
	row := t.tx.QueryRowContext(t.ctx,
		`SELECT id, owner, idempotency_key, from_account, to_account, amount, currency, created_at
		 FROM transfers WHERE id = ?`, id)

	tr, err := scanTransfer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Transfer{}, false, nil
	}
	if err != nil {
		return core.Transfer{}, false, fmt.Errorf("find transfer by id: %w", err)
	}
	postings, perr := t.postingsOf(tr.ID)
	if perr != nil {
		return core.Transfer{}, false, perr
	}
	tr.Postings = postings
	return tr, true, nil
}

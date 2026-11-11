package core

import "context"

// Store is the storage-agnostic port (00-core.md §4b). core NEVER imports a DB
// driver; it talks only to Store. InTx is the serialization primitive (§5): the
// SQLite adapter runs fn inside a single BEGIN IMMEDIATE write transaction, so
// every read fn makes is held under the write lock and the whole closure commits
// or rolls back atomically.
type Store interface {
	InTx(ctx context.Context, fn func(tx Tx) error) error

	// Read-only paths (may run outside a write tx).
	GetAccount(ctx context.Context, id AccountID) (Account, error)
	AccountsByOwner(ctx context.Context, owner PrincipalID) ([]Account, error)
	Balance(ctx context.Context, id AccountID) (Amount, error)
	Statement(ctx context.Context, id AccountID, page Page) (StatementPage, error)
}

// Tx is the transactional surface. It exposes INSERTs and reads but DELIBERATELY
// NO update or delete of postings - append-only (R5) is enforced by the absence of
// a code path, not by convention. InsertTransfer writes the transfer row and all
// its postings in one call (assigning each posting's seq as MAX(seq)+1 per account
// under the write lock, D-B1) so the adapter keeps them in one SQL transaction
// frame.
type Tx interface {
	GetAccount(id AccountID) (Account, error)
	Balance(id AccountID) (Amount, error) // live, under the write lock
	InsertAccount(a Account) error
	FindTransferByKey(owner PrincipalID, key string) (Transfer, bool, error) // R4 lookup
	InsertTransfer(t Transfer) error                                         // writes transfer + postings atomically
}

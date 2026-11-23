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
	ListCurrencies(ctx context.Context) ([]CurrencyMeta, error)
	// ActiveHoldsTotal sums the amounts of an account's active holds (read-only).
	ActiveHoldsTotal(ctx context.Context, id AccountID) (Amount, error)
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

	// FindTransferByID loads a transfer and its postings by id, under the write
	// lock. Reversal needs it twice: to read the entry being compensated, and to
	// detect an existing compensating entry by its derived id (see core/reversal.go).
	// Still no update or delete - append-only (R5) is unchanged.
	FindTransferByID(id string) (Transfer, bool, error)

	// Hold surface. ActiveHoldsTotal is the live reserved sum under the write lock;
	// FindHoldByKey is the idempotency lookup; GetHold loads a hold for release.
	ActiveHoldsTotal(id AccountID) (Amount, error)
	FindHoldByKey(owner PrincipalID, key string) (Hold, bool, error)
	InsertHold(h Hold) error
	GetHold(id HoldID) (Hold, bool, error)
	ReleaseHold(id HoldID) error // marks the hold inactive (idempotent on an active hold)
}

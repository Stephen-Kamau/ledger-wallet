package core

import (
	"context"
	"time"
)

// Ledger is the entire domain surface api is allowed to touch (00-core.md §4a).
// api does auth and transport; everything financial happens behind this interface.
// All errors are typed (errors.go, R13). FROZEN contract.
type Ledger interface {
	// CreateAccount mints an account for an owner in a currency (R6, R9).
	CreateAccount(ctx context.Context, cmd CreateAccountCmd) (Account, error)

	// GetAccount returns the account record (R9); carries Currency and Owner so
	// api composes a {amount, currency} balance body. ErrAccountNotFound if unknown.
	GetAccount(ctx context.Context, id AccountID) (Account, error)

	// AccountsByOwner returns every account owned by a principal (R20, D-C1).
	AccountsByOwner(ctx context.Context, owner PrincipalID) ([]Account, error)

	// Balance returns the derived balance (sum of postings) for an account (R10).
	Balance(ctx context.Context, id AccountID) (Amount, error)

	// Transfer performs a balanced, authorised, idempotent transfer (R1-R8, R11).
	// Replaying an idempotency key returns the original result with no second
	// posting (R4).
	Transfer(ctx context.Context, cmd TransferCmd) (TransferResult, error)

	// Statement returns an account's postings in Seq order, paged (R12).
	Statement(ctx context.Context, id AccountID, page Page) (StatementPage, error)

	// PlaceHold reserves Amount on Account without posting (an account hold). The
	// authorizer must own the account (R7, reused). It reduces the AVAILABLE balance
	// and is rejected if available (balance minus active holds) would go negative on a
	// non-AllowNegative account (ErrInsufficientFunds). Idempotent per (owner, key).
	PlaceHold(ctx context.Context, cmd HoldCmd) (Hold, error)

	// ReleaseHold releases an active hold, freeing the reserved funds. The authorizer
	// must own the held account (R7). An unknown or already-released hold returns
	// ErrHoldNotFound.
	ReleaseHold(ctx context.Context, authorizer PrincipalID, id HoldID) error

	// AvailableBalance is the posted balance minus the sum of active holds.
	AvailableBalance(ctx context.Context, id AccountID) (Amount, error)
}

// ledger is the concrete invariant engine over a Store. It is the only writer of
// postings; all of R1-R8 are enforced here, in Transfer.
type ledger struct {
	store Store
	now   func() time.Time
}

// New constructs the engine. This is the only wiring point; main() passes the
// SQLite Store.
func New(store Store) Ledger {
	return &ledger{store: store, now: func() time.Time { return time.Now().UTC() }}
}

// CreateAccount is a thin single-INSERT path: the only invariant is that owner and
// currency are set (R6). core mints the opaque AccountID.
func (l *ledger) CreateAccount(ctx context.Context, cmd CreateAccountCmd) (Account, error) {
	if cmd.Owner == "" || cmd.Currency == "" {
		return Account{}, ErrInvalidAccount
	}
	a := Account{
		ID:            AccountID(newID("acct")),
		Owner:         cmd.Owner,
		Currency:      cmd.Currency,
		AllowNegative: cmd.AllowNegative,
		CreatedAt:     l.now(),
	}
	err := l.store.InTx(ctx, func(tx Tx) error {
		return tx.InsertAccount(a)
	})
	if err != nil {
		return Account{}, err
	}
	return a, nil
}

// GetAccount returns the account record or ErrAccountNotFound (R9).
func (l *ledger) GetAccount(ctx context.Context, id AccountID) (Account, error) {
	return l.store.GetAccount(ctx, id)
}

// AccountsByOwner returns the owner's accounts (R20, D-C1).
func (l *ledger) AccountsByOwner(ctx context.Context, owner PrincipalID) ([]Account, error) {
	return l.store.AccountsByOwner(ctx, owner)
}

// Balance returns the derived balance (R10); read-only, outside the write tx.
func (l *ledger) Balance(ctx context.Context, id AccountID) (Amount, error) {
	return l.store.Balance(ctx, id)
}

// Statement returns the account's postings keyset-paged on Seq (R12); read-only.
func (l *ledger) Statement(ctx context.Context, id AccountID, page Page) (StatementPage, error) {
	if page.Limit <= 0 {
		page.Limit = defaultPageLimit
	}
	if page.Limit > maxPageLimit {
		page.Limit = maxPageLimit
	}
	return l.store.Statement(ctx, id, page)
}

// Transfer performs a balanced, authorised, idempotent transfer. The ordering
// matters (00-core.md §3): cheap caller-visible rejections (R1, R8, R7) happen
// before the transaction opens; fund sufficiency (R3) and the idempotency replay
// (R4) happen inside it, under the write lock, because they depend on live state.
func (l *ledger) Transfer(ctx context.Context, cmd TransferCmd) (TransferResult, error) {
	// --- pre-transaction: cheap, pure, no writes ---
	entries, err := buildEntries(cmd) // R1 + positive magnitude
	if err != nil {
		return TransferResult{}, err
	}

	from, err := l.store.GetAccount(ctx, cmd.From)
	if err != nil {
		return TransferResult{}, err // ErrAccountNotFound
	}
	to, err := l.store.GetAccount(ctx, cmd.To)
	if err != nil {
		return TransferResult{}, err
	}

	if from.Currency != to.Currency || from.Currency != cmd.Currency { // R8
		return TransferResult{}, ErrCurrencyMismatch
	}
	if from.Owner != cmd.Authorizer { // R7 - before any write
		return TransferResult{}, ErrUnauthorized
	}

	// --- transaction: serialized writer; R4 then R3 under the write lock (§5) ---
	var result TransferResult
	err = l.store.InTx(ctx, func(tx Tx) error {
		prior, ok, ferr := tx.FindTransferByKey(cmd.Authorizer, cmd.IdempotencyKey)
		if ferr != nil {
			return ferr
		}
		if ok { // R4: replay, no second post
			if !sameParams(prior, cmd) {
				return ErrIdempotencyConflict
			}
			result = TransferResult{Transfer: prior, Replayed: true}
			return nil
		}

		if !from.AllowNegative { // R3: live balance under the held write lock
			bal, berr := tx.Balance(cmd.From)
			if berr != nil {
				return berr
			}
			if bal+debitLeg(entries) < 0 {
				return ErrInsufficientFunds
			}
		}

		t := l.materialise(cmd, entries) // builds Transfer + its Postings (seq filled by adapter)
		if ierr := tx.InsertTransfer(t); ierr != nil {
			return ierr
		}
		result = TransferResult{Transfer: t, Replayed: false}
		return nil
	})
	if err != nil {
		return TransferResult{}, err
	}
	return result, nil
}

// materialise builds the Transfer record and its balanced postings from a command
// and its validated entries. Posting.Seq is left zero: the storage adapter assigns
// it as MAX(seq)+1 per account inside the write transaction (D-B1).
func (l *ledger) materialise(cmd TransferCmd, entries []Entry) Transfer {
	now := l.now()
	tid := newID("xfer")
	postings := make([]Posting, 0, len(entries))
	for _, e := range entries {
		postings = append(postings, Posting{
			ID:         newID("post"),
			TransferID: tid,
			Account:    e.Account,
			Amount:     e.Amount,
			Currency:   cmd.Currency,
			CreatedAt:  now,
		})
	}
	return Transfer{
		ID:             tid,
		IdempotencyKey: cmd.IdempotencyKey,
		Owner:          cmd.Authorizer,
		From:           cmd.From,
		To:             cmd.To,
		Amount:         cmd.Amount,
		Currency:       cmd.Currency,
		CreatedAt:      now,
		Postings:       postings,
	}
}

// PlaceHold reserves funds without posting. The ordering mirrors Transfer: cheap,
// pure, caller-visible rejections (positive magnitude, currency, ownership) happen
// before the transaction opens; the idempotency replay and the available-funds check
// happen inside it, under the write lock, because they depend on live state.
func (l *ledger) PlaceHold(ctx context.Context, cmd HoldCmd) (Hold, error) {
	// --- pre-transaction: cheap, pure, no writes ---
	if cmd.Amount <= 0 { // a hold reserves a positive magnitude
		return Hold{}, ErrUnbalanced
	}
	acct, err := l.store.GetAccount(ctx, cmd.Account)
	if err != nil {
		return Hold{}, err // ErrAccountNotFound
	}
	if acct.Currency != cmd.Currency { // R8, reused
		return Hold{}, ErrCurrencyMismatch
	}
	if acct.Owner != cmd.Authorizer { // R7, reused - before any write
		return Hold{}, ErrUnauthorized
	}

	// --- transaction: serialized writer; replay then sufficiency under the lock ---
	var result Hold
	err = l.store.InTx(ctx, func(tx Tx) error {
		prior, ok, ferr := tx.FindHoldByKey(cmd.Authorizer, cmd.IdempotencyKey)
		if ferr != nil {
			return ferr
		}
		if ok { // replay: no second reservation
			if !sameHoldParams(prior, cmd) {
				return ErrIdempotencyConflict
			}
			result = prior
			return nil
		}

		if !acct.AllowNegative { // available (balance - active holds) must stay >= 0
			bal, berr := tx.Balance(cmd.Account)
			if berr != nil {
				return berr
			}
			held, herr := tx.ActiveHoldsTotal(cmd.Account)
			if herr != nil {
				return herr
			}
			if bal-held-cmd.Amount < 0 {
				return ErrInsufficientFunds
			}
		}

		h := Hold{
			ID:             HoldID(newID("hold")),
			Account:        cmd.Account,
			Owner:          cmd.Authorizer,
			Amount:         cmd.Amount,
			Currency:       cmd.Currency,
			IdempotencyKey: cmd.IdempotencyKey,
			Active:         true,
			CreatedAt:      l.now(),
		}
		if ierr := tx.InsertHold(h); ierr != nil {
			return ierr
		}
		result = h
		return nil
	})
	if err != nil {
		return Hold{}, err
	}
	return result, nil
}

// ReleaseHold frees an active hold's funds. The lookup, the ownership check and the
// release all happen under the write lock so a concurrent place/release cannot race.
func (l *ledger) ReleaseHold(ctx context.Context, authorizer PrincipalID, id HoldID) error {
	return l.store.InTx(ctx, func(tx Tx) error {
		h, ok, err := tx.GetHold(id)
		if err != nil {
			return err
		}
		if !ok || !h.Active { // unknown or already released
			return ErrHoldNotFound
		}
		if h.Owner != authorizer { // R7, reused: only the owner may release
			return ErrUnauthorized
		}
		return tx.ReleaseHold(id)
	})
}

// AvailableBalance is the posted balance minus the sum of active holds (read-only).
func (l *ledger) AvailableBalance(ctx context.Context, id AccountID) (Amount, error) {
	bal, err := l.store.Balance(ctx, id) // also the ErrAccountNotFound guard
	if err != nil {
		return 0, err
	}
	held, err := l.store.ActiveHoldsTotal(ctx, id)
	if err != nil {
		return 0, err
	}
	return bal - held, nil
}

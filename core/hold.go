package core

import "time"

// HoldID is an opaque id minted by core for a reservation.
type HoldID string

// Hold is a reservation of funds on an account that has NOT been posted. It reduces
// the account's AVAILABLE balance (balance minus active holds) without writing any
// posting, so the posted balance (R10) is untouched while the hold is active. A hold
// is released explicitly (ReleaseHold), which frees the reserved funds. Holds are the
// idempotency unit for PlaceHold (a UNIQUE (owner, idempotency_key), like transfers).
type Hold struct {
	ID             HoldID
	Account        AccountID
	Owner          PrincipalID // the authorising principal who placed it (R7)
	Amount         Amount      // the positive magnitude reserved
	Currency       Currency
	IdempotencyKey string // client-supplied, unique per owner
	Active         bool   // true until released
	CreatedAt      time.Time
	ReleasedAt     time.Time // zero while active; set when released
}

// HoldCmd is the inbound command for PlaceHold. api sets Authorizer to the
// AUTHENTICATED caller; core checks ownership of the held account (R7, reused).
type HoldCmd struct {
	Account        AccountID
	Authorizer     PrincipalID // the authenticated caller (R7)
	Amount         Amount      // positive magnitude to reserve
	Currency       Currency
	IdempotencyKey string // client-supplied; required
}

// sameHoldParams reports whether a prior hold matches a replayed command's
// parameters. A key reused with different params is a conflict (mirrors transfers).
func sameHoldParams(prior Hold, cmd HoldCmd) bool {
	return prior.Account == cmd.Account &&
		prior.Amount == cmd.Amount &&
		prior.Currency == cmd.Currency &&
		prior.Owner == cmd.Authorizer
}

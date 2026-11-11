// Package core is the double-entry ledger's invariant engine. It is the only
// package in the module that carries domain meaning, and it depends on nothing
// but the standard library: it defines the Store port (store.go) and talks to
// storage only through it, so the engine is unit-testable with a fake Store and
// the SQLite adapter is swappable. All enforcement (R1-R8) lives here.
package core

import "time"

// Amount is a signed quantity in the smallest unit of its currency (e.g. cents).
// A debit is negative, a credit is positive. Amounts only have meaning paired
// with a Currency; core never mixes currencies within a transfer (R8). Money is
// integer minor units throughout - no float ever touches a balance, because
// floating point is not associative and a ledger that is not associative is not
// conserved.
type Amount int64

// Currency is an ISO-ish label, opaque to core ("USD", "GBP"). It is a dumb
// scale label for the toy; core does no conversion (R8) and validates it only
// as non-empty.
type Currency string

// AccountID is an opaque id minted by core; it is never the owner.
type AccountID string

// PrincipalID is the owning principal, the auth subject (R6, R7).
type PrincipalID string

// Account is an owned balance holder. It stores NO balance field - the balance
// is derived from postings (R10). AllowNegative is the single, explicit escape
// hatch for the system/mint account (R3); every other account is floored at zero.
type Account struct {
	ID            AccountID
	Owner         PrincipalID
	Currency      Currency
	AllowNegative bool // true only for designated system/mint accounts
	CreatedAt     time.Time
}

// CreateAccountCmd mints an account for an owner in a currency (R6, R9).
// AllowNegative is for system/mint provisioning only; the api layer gates who
// may set it (00-core.md open question 7).
type CreateAccountCmd struct {
	Owner         PrincipalID
	Currency      Currency
	AllowNegative bool
}

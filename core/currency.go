package core

// CurrencyMeta is the public reference metadata for a currency code, carried by
// migration 0002: the minor-unit scale (decimal places - 2 => cents, 0 => yen) and
// the display symbol a UI needs to render money correctly (R20/R22) without ever
// parsing money as a float.
//
// It is DISTINCT from Currency (account.go), which is the opaque scale label stamped
// on accounts and transfers. Currency is the dumb "USD" tag core validates only as
// non-empty; CurrencyMeta is the reference row keyed by that tag. The names differ
// because Currency is already the string label and a struct cannot reuse the name.
type CurrencyMeta struct {
	Code      string
	MinorUnit int
	Symbol    string
}

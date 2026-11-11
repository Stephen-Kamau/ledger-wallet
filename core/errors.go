package core

// LedgerError is a typed domain error (R13). Its Code is a stable string that the
// api layer maps to an HTTP status; the Code strings are frozen with the contract.
type LedgerError struct {
	Code    string
	Message string
}

func (e *LedgerError) Error() string { return e.Code + ": " + e.Message }

// Is lets errors.Is match sentinels by Code, so wrapped or freshly-built
// LedgerError values with the same Code compare equal.
func (e *LedgerError) Is(target error) bool {
	t, ok := target.(*LedgerError)
	return ok && t.Code == e.Code
}

// Typed sentinel errors (R13). Each invariant violation returns a distinct Code;
// api never surfaces a bare 500 for a known rule. Proposed status mapping lives in
// 00-core.md open question 2 and is Dakari's to own.
var (
	// ErrUnbalanced - the transfer's legs do not net to exactly zero (R1) -> 422.
	ErrUnbalanced = &LedgerError{Code: "unbalanced", Message: "transfer legs do not sum to zero"}
	// ErrInsufficientFunds - a normal account would go below zero (R3) -> 409.
	ErrInsufficientFunds = &LedgerError{Code: "insufficient_funds", Message: "insufficient funds for debit"}
	// ErrUnauthorized - the authorizer does not own the debited account (R7) -> 403.
	ErrUnauthorized = &LedgerError{Code: "unauthorized", Message: "authorizer does not own the debited account"}
	// ErrCurrencyMismatch - legs or accounts disagree on currency (R8) -> 422.
	ErrCurrencyMismatch = &LedgerError{Code: "currency_mismatch", Message: "all legs must share one currency"}
	// ErrAccountNotFound - an unknown account id -> 404.
	ErrAccountNotFound = &LedgerError{Code: "account_not_found", Message: "account not found"}
	// ErrIdempotencyConflict - a key reused with different parameters (R4) -> 409.
	ErrIdempotencyConflict = &LedgerError{Code: "idempotency_conflict", Message: "idempotency key reused with different parameters"}
	// ErrInvalidAccount - a create command missing an owner or currency (R6) -> 422.
	ErrInvalidAccount = &LedgerError{Code: "invalid_account", Message: "account requires an owner and a currency"}
)

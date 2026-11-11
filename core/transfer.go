package core

import "time"

// Transfer is the unit of intent and the idempotency unit (R4). It records the
// materialised movement; its Postings are the proof it balanced (R1).
type Transfer struct {
	ID             string
	IdempotencyKey string      // client-supplied, unique per owner (R4)
	Owner          PrincipalID // the authorising principal (R7)
	From, To       AccountID
	Amount         Amount // the positive magnitude moved (From->To)
	Currency       Currency
	CreatedAt      time.Time
	Postings       []Posting // exactly the balanced legs that were written
}

// TransferCmd is the inbound command for a transfer. api sets Authorizer to the
// AUTHENTICATED caller; core checks ownership of the debited account (R7).
type TransferCmd struct {
	Authorizer     PrincipalID // the authenticated caller (R7)
	From, To       AccountID
	Amount         Amount // positive magnitude moved From->To
	Currency       Currency
	IdempotencyKey string // client-supplied (R4); required
}

// TransferResult is the outcome of a transfer. Replayed is true when the result
// came from an idempotency replay rather than a fresh write (R4).
type TransferResult struct {
	Transfer Transfer
	Replayed bool
}

// Page is keyset pagination on Posting.Seq (R12).
type Page struct {
	Limit    int
	AfterSeq int64
}

// StatementPage is a page of postings in Seq order plus the keyset cursor (R12).
type StatementPage struct {
	Postings     []Posting
	NextAfterSeq int64
	HasMore      bool
}

// defaultPageLimit bounds a statement page when the caller supplies none.
const defaultPageLimit = 50

// maxPageLimit caps an over-large caller request.
const maxPageLimit = 500

// buildEntries turns a TransferCmd into the two balanced legs it implies and
// enforces R1 (the legs sum to exactly zero) and the positive-magnitude rule.
// It is pure and writes nothing, so it runs before the transaction opens.
func buildEntries(cmd TransferCmd) ([]Entry, error) {
	if cmd.Amount <= 0 {
		return nil, ErrUnbalanced
	}
	if cmd.From == cmd.To {
		// A self-transfer still nets to zero but moves nothing and would write
		// two postings to one account; reject it as unbalanced intent.
		return nil, ErrUnbalanced
	}
	entries := []Entry{
		{Account: cmd.From, Amount: -cmd.Amount}, // debit
		{Account: cmd.To, Amount: cmd.Amount},    // credit
	}
	var sum Amount
	for _, e := range entries {
		sum += e.Amount
	}
	if sum != 0 { // R1: must net to exactly zero
		return nil, ErrUnbalanced
	}
	return entries, nil
}

// debitLeg returns the signed debit total of the entries (the negative sum).
func debitLeg(entries []Entry) Amount {
	var d Amount
	for _, e := range entries {
		if e.Amount < 0 {
			d += e.Amount
		}
	}
	return d
}

// sameParams reports whether a prior transfer matches the replayed command's
// financial parameters. A key reused with different params is a conflict (R4).
func sameParams(prior Transfer, cmd TransferCmd) bool {
	return prior.From == cmd.From &&
		prior.To == cmd.To &&
		prior.Amount == cmd.Amount &&
		prior.Currency == cmd.Currency &&
		prior.Owner == cmd.Authorizer
}

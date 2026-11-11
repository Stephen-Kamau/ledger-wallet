package core

import "time"

// Entry is an INTENDED leg of a transfer - the request-level unit. A transfer is
// a set of entries that must net to zero (R1). Entries are validated, then
// persisted as Postings. The separation of Entry (intent) from Posting (fact) is
// what lets the engine validate and reject before anything is written (R1, R7).
type Entry struct {
	Account AccountID
	Amount  Amount // signed: negative debits, positive credits
}

// Posting is the PERSISTED, immutable record of an applied leg (R5). Once written
// it is never updated or deleted - a correction is a new compensating transfer.
// Seq gives a total order within an account for statements (R12); it is assigned
// by the storage adapter inside the write transaction (D-B1), not by core.
type Posting struct {
	ID         string
	TransferID string
	Account    AccountID
	Amount     Amount
	Currency   Currency
	Seq        int64 // monotonic per-account ordering for statements
	CreatedAt  time.Time
}

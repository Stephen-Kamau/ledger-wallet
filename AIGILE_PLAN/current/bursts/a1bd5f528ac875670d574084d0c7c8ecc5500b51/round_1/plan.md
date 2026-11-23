<!-- AIGILE-RECORD
kind: plan
stage: plan
round: 1
schema_version: 1.1.0
superseded_by: none
-->

# Stream plan - amara - burst `reversals`

Reversal of a ledger entry as a COMPENSATING entry (R5), built from the invariant up.
Nothing is ever mutated or deleted; the reversed entry stays in the ledger and in the
statement.

Supersedes round_1 plan r1: the adapter's `core.Tx` implementation lives on the `txn`
type in `internal/sqlite/tx.go`, which r1 failed to declare. The new Tx method is added
in a new `internal/sqlite/reversal.go` so `tx.go` itself stays untouched; both are
declared below rather than written outside the claim.

- Scope (files WRITTEN):
  - `core/ledger.go`
  - `core/store.go`
  - `core/errors.go`
  - `core/reversal.go` (new)
  - `core/reversal_test.go` (new)
  - `core/fake_store_test.go`
  - `internal/sqlite/reversal.go` (new)
  - `internal/sqlite/reversal_test.go` (new)

## Design

`Reverse(ctx, authorizer, entryID, idempotencyKey) (TransferResult, error)` on `Ledger`.
The authorizer is positional, mirroring the existing `ReleaseHold(ctx, authorizer, id)`;
the pack's `Reverse(ctx, entryID, idempotencyKey)` carries no principal and so could not
enforce R7.

- The compensating entry mirrors the ORIGINAL postings (each amount negated), so its own
  postings sum to zero (R1) and total conservation is unchanged (R2).
- Append-only (R5): no UPDATE, no DELETE. The append-only triggers stay untouched.
- At most one reversal per entry, enforced by a DERIVED primary key `rev_<entryID>` on the
  reversal's transfer row. A second attempt is `ErrAlreadyReversed` (R13), not a second
  compensating entry. This needs NO schema change, so it cannot reintroduce the
  `embeddedSchema`-vs-`migrations/` drift that bit `collision-v2`, and `migrations/` stays
  outside this stream's scope.
- R7 is enforced on the account the reversal DEBITS (the original's destination).
- R3 is enforced on that same account unless it is `AllowNegative`.
- R4 replay on (owner, idempotencyKey) returns the original result, never a second post.

## Known cross-stream effect

Adding a method to `core.Ledger` and `core.Tx` breaks every implementer at compile time,
including `api/fakes_test.go` (`fakeLedger`, `memTx`) which is dakari's file and NOT
claimed here. `go build ./...` is unaffected; `go test ./api/...` cannot compile until
dakari's side lands. Reported rather than silently patched.

## Verification

- Unit tests over the fake store for each invariant and each typed error.
- A REAL-ADAPTER test in `internal/sqlite/` as well: both prior bursts shipped a defect the
  fakes hid (`build-v1` C-1 seq-not-in-result, `collision-v2` embeddedSchema drift), so the
  invariants are asserted against SQLite, including that the append-only triggers still hold.

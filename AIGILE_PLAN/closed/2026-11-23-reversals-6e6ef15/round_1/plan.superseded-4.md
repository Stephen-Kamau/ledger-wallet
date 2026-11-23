<!-- AIGILE-RECORD
kind: plan
stage: plan
round: 1
schema_version: 1.1.0
superseded_by: AIGILE_PLAN/current/bursts/58487fc04e63b14d4067564b4902dad8da2d0238/round_1/plan.md
-->

## Burst Plan

Stream plan - kofi (Kofi Otieno), burst `reversals`. Rung: builder.
Stream: schema + the concurrency proof.

- Scope (files WRITTEN):
  - `migrations/` (the 0004_reversals migration)
  - `tests/` (bootstrap parity, boot-path drift guard, reversal concurrency)

### Instruction

Operator pack `operator-prompts/kofi.md`, burst `reversals`. Three clauses:

1. Migration `0004_reversals` recording the link between an entry and its reversal,
   numbered and idempotent (R14, a CHARTER Hard Spec), with DB-level uniqueness that
   makes a double reversal impossible in the database, not only in Go.
2. The drift trap: `collision-v2` shipped a 500 because `cmd/ledgerd` booted an embedded
   schema a migration had never reached. Verify 0004 reaches the WIRED boot path and add
   a regression test that FAILS on the old wiring. Bootstrap parity is R16.
3. Concurrency tests (R17, R18) under `-race`: concurrent reversals of the same entry
   yield exactly one compensating entry; concurrent reversals of different entries do
   not serialize into uselessness; conservation (sum of postings = 0) holds.

### Which stream discharges each clause of the instruction

| Clause | Stream | Requirement | Artefact |
| --- | --- | --- | --- |
| 1 | kofi | 0004_reversals, numbered + idempotent (R14) | `migrations/0004_reversals.sql` |
| 1 | kofi | DB-level uniqueness forbidding a double reversal | `UNIQUE (original_transfer_id)` in 0004 |
| 2 | kofi | 0004 reaches the WIRED boot path | `tests/bootpath_test.go` - boots the real `cmd/ledgerd` binary |
| 2 | kofi | Regression test that FAILS on the old wiring | same test: an `embeddedSchema` boot has no reversals objects |
| 2 | kofi | Bootstrap parity (R16) extended to 0004 | `tests/migrations_test.go` evolved path |
| 3 | kofi | R18: concurrent reversals of the SAME entry -> exactly one | `tests/reversals_test.go` |
| 3 | kofi | R17: concurrent reversals of DIFFERENT entries all land | `tests/reversals_test.go` |
| 3 | kofi | Conservation (sum of postings = 0) under `-race` | `assertInvariants` in `tests/harness_test.go` |
| 3 | none, `core/**` is another operator's scope - reported to the owner, not built | `core.Reverse` / `Tx.InsertReversal` seam | n/a |

### Stream kofi

- Executor: builder
- Work class: implement
- Scope (files WRITTEN):
  - `migrations/` (the 0004_reversals migration)
  - `tests/` (bootstrap parity, boot-path drift guard, reversal concurrency)

The only stream in this clone. The other four operators plan their own streams in their
own clones; this record deliberately names none of their files, so the signed claim it
backs cannot collide with theirs.

## How it works

`0004_reversals.sql` adds one table, `reversals`, the link record between an original
transfer and the compensating transfer that reverses it. Additive only: it touches no
existing table, so bootstrap parity (R16) stays trivially true and this is not a chain
event. Every object is `CREATE ... IF NOT EXISTS`, so re-running is a no-op (R14).

The hard guarantee is `UNIQUE (original_transfer_id)`. A second reversal of the same
entry is not rejected by a Go branch that a race can slip past - it is refused by SQLite
at INSERT time, so exactly one of N concurrent attempts can ever win.

Because `core` exposes no seam to write the link inside its own write transaction, the
row is written in two phases:

- **claim** - insert the link with `reversal_transfer_id` NULL. The UNIQUE constraint
  decides the single winner, atomically, before any posting happens.
- **settle** - once the compensating transfer has posted, fill in
  `reversal_transfer_id` and `settled_at`.

A partial UNIQUE index on `reversal_transfer_id WHERE NOT NULL` makes a compensating
transfer settle at most one reversal, and a `CHECK` keeps the two settle columns set or
unset together. Triggers forbid repointing `original_transfer_id` and overwriting an
already-settled `reversal_transfer_id`, which is the R5 append-only posture applied to
the link record.

## Integration

- **`migrations.Apply`** picks up `0004_reversals.sql` automatically - the applier
  embeds `*.sql` and sorts on the four-digit prefix, so no Go change is needed.
- **`cmd/ledgerd/wire.go`** already applies migrations as the single schema source, so
  the wired server gets the reversals objects for free. `tests/bootpath_test.go` proves
  it against the real binary rather than trusting the source comment.
  `cmd/ledgerd/wire_test.go` covers the pre-0004 tables but is NOT in this stream's
  scope, so the new guard lives in `tests/`.
- **`internal/sqlite`'s `embeddedSchema`** is adapter-test-only and is NOT in scope. It
  will not carry the reversals table; that is now harmless because the wired path no
  longer boots from it, and the boot-path guard is what keeps it harmless.
- **`core` / `api`** belong to other operators. This stream adds no seam to either.

## Alternatives considered and discarded

- **Single-phase link insert (`reversal_transfer_id` NOT NULL).** Discarded: the
  compensating transfer id does not exist until after `core.Ledger.Transfer` returns, so
  a NOT NULL column forces the link to be written after the posting - which is exactly
  the window a double reversal races through. Claim-first is what makes the constraint
  load-bearing.
- **Relying on R4 idempotency (`UNIQUE (owner, idempotency_key)` on transfers) with a
  key derived from the original transfer id.** Discarded as the primary guarantee: it
  works only while every caller agrees to derive the key the same way, which is a
  convention, not a database constraint. The pack asks for uniqueness in the database.
  The tests therefore use DISTINCT idempotency keys per goroutine, so only the reversals
  constraint can prevent the second posting.
- **Adding `Tx.InsertReversal` to `core` so claim and posting share one transaction.**
  The right end state, and discarded here only because `core/**` is another operator's
  scope. Reported to the owner instead of taken.
- **A `reversed_by` column on `transfers`.** Discarded: `ALTER TABLE ... ADD COLUMN`
  cannot add a UNIQUE constraint in SQLite, and a partial unique index over a nullable
  column on the transfers table would entangle the reversal concept with the table every
  other stream writes. A separate table keeps the migration additive and the ownership
  boundary clean.

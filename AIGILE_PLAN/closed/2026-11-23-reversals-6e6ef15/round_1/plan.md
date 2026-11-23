<!-- AIGILE-RECORD
kind: plan
stage: plan
round: 1
schema_version: 1.1.0
superseded_by: none
-->

<!-- SECTION: plan/do -->

## Burst Plan

Stream plan - kofi (Kofi Otieno), burst `reversals`. Rung: builder.

**Goal:** migration `0004_reversals` - the link between an entry and its reversal, with
the no-double-reversal guarantee enforced by SQLite rather than by Go - plus the
concurrency proof and the boot-path drift guard.

**Streams:** kofi

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

| Instruction clause | Discharged by | Artefact |
| --- | --- | --- |
| 1 - 0004_reversals, numbered and idempotent (R14) | kofi | `migrations/0004_reversals.sql` |
| 1 - DB-level uniqueness forbidding a double reversal | kofi | `UNIQUE (original_transfer_id)` in 0004 |
| 2 - 0004 reaches the WIRED boot path | kofi | `tests/bootpath_test.go`, which boots the real `cmd/ledgerd` binary |
| 2 - a regression test that FAILS on the old wiring | kofi | the same test: an `embeddedSchema` boot carries no reversals objects |
| 2 - bootstrap parity (R16) extended to 0004 | kofi | the evolved path in `tests/migrations_test.go` |
| 3 - R18, concurrent reversals of the SAME entry yield exactly one | kofi | `tests/reversals_test.go` |
| 3 - R17, concurrent reversals of DIFFERENT entries all land | kofi | `tests/reversals_test.go` |
| 3 - conservation, sum of postings = 0, under `-race` | kofi | `assertInvariants` in `tests/harness_test.go` |
| a `core.Reverse` / `Tx.InsertReversal` seam | none, `core/**` is another operator's scope and the pack says report a missing seam rather than add it | reported to the core owner |

### Stream kofi - schema and the concurrency proof

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

<!-- SECTION: plan/why -->

The hard guarantee is `UNIQUE (original_transfer_id)`, and it is non-negotiable that it
lives in the database. A Go-side "has this been reversed?" check is a read followed by a
write, and two goroutines interleave inside that gap - which is precisely the shape of
bug R18 exists to forbid. A UNIQUE constraint collapses the check and the write into one
atomic act: of N concurrent attempts, SQLite lets exactly one INSERT succeed and refuses
the rest, with no cooperation required from the caller.

<!-- /SECTION: plan/why -->

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
  that against the real binary rather than trusting the source comment.
  `cmd/ledgerd/wire_test.go` covers the pre-0004 tables but is NOT in this stream's
  scope, so the new guard lives in `tests/`.
- **`internal/sqlite`'s `embeddedSchema`** is adapter-test-only and is NOT in scope. It
  will not carry the reversals table; that is harmless because the wired path no longer
  boots from it, and the boot-path guard is what keeps it harmless.
- **`core` / `api`** belong to other operators. This stream adds no seam to either.

<!-- /SECTION: plan/do -->

<!-- SECTION: plan/provenance -->

## Alternatives considered and discarded

- **Single-phase link insert, `reversal_transfer_id NOT NULL`.** Discarded: the
  compensating transfer's id does not exist until after `core.Ledger.Transfer` returns,
  so a NOT NULL column forces the link to be written after the posting - which is
  exactly the window a double reversal races through. Claim-first is what makes the
  constraint load-bearing.
- **Relying on R4 idempotency (`UNIQUE (owner, idempotency_key)` on transfers) with a
  key derived from the original transfer id.** Discarded as the primary guarantee: it
  holds only while every caller agrees to derive the key the same way, which is a
  convention, not a database constraint. The pack asks for uniqueness in the database.
  The tests therefore use DISTINCT idempotency keys per goroutine, so only the reversals
  constraint can prevent the second posting.
- **Adding `Tx.InsertReversal` to `core` so the claim and the posting share one
  transaction.** The right end state, and discarded here only because `core/**` is
  another operator's scope. Reported to the owner instead of taken.
- **A `reversed_by` column on `transfers`.** Discarded: SQLite's `ALTER TABLE ... ADD
  COLUMN` cannot add a UNIQUE constraint, and a partial unique index over a nullable
  column on `transfers` would entangle the reversal concept with the table every other
  stream writes. A separate table keeps the migration additive and the ownership
  boundary clean.

<!-- /SECTION: plan/provenance -->

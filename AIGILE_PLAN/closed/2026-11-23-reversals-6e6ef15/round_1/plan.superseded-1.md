<!-- AIGILE-RECORD
kind: plan
stage: plan
round: 1
schema_version: 1.1.0
superseded_by: AIGILE_PLAN/current/bursts/58487fc04e63b14d4067564b4902dad8da2d0238/round_1/plan.md
-->

# Stream plan - kofi (burst `reversals`)

Operator: kofi (Kofi Otieno). Rung: builder. Stream: schema + concurrency proof.

- Scope (files WRITTEN):
  - `migrations/` (the 0004_reversals migration)
  - `tests/` (bootstrap parity, boot-path drift guard, reversal concurrency)

## Intent

Add migration `0004_reversals`: the link record between an original transfer and its
compensating reversal, with DB-level uniqueness that makes a double reversal impossible
in SQLite, not merely in Go. Then prove it under `-race`.

## Deliverables

1. `migrations/0004_reversals.sql` - numbered and idempotent (R14, CHARTER Hard Spec).
   Additive only, so bootstrap parity (R16) stays trivially true. The hard guarantee is
   `UNIQUE (original_transfer_id)`: at most one reversal per entry, enforced by the
   database.
2. Boot-path drift guard in `tests/` - the `collision-v2` 500 happened because
   `cmd/ledgerd` booted the adapter's `embeddedSchema`, which a migration had never
   reached. `migrations.Apply` is the single schema source now. The guard boots the REAL
   wired `cmd/ledgerd` binary against a temp DB and asserts the reversals objects are
   present, so it fails closed on the old wiring. `cmd/ledgerd/wire_test.go` already
   covers this for the pre-0004 tables but is NOT in this stream's scope, so the new
   guard lives in `tests/`.
3. Bootstrap parity (R16) extended to 0004 in the step-by-step evolved path.
4. Concurrency proof under `-race` (R17, R18):
   - concurrent reversals of the SAME entry yield exactly one compensating entry;
   - concurrent reversals of DIFFERENT entries all succeed (the constraint does not
     serialize into uselessness);
   - conservation holds throughout: sum of all postings = 0.

## Boundaries

`core/**` and `api/**` belong to other operators. core exposes no `Reverse` seam and no
`Tx.InsertReversal`, so the reversal link cannot yet be written inside core's write
transaction. This stream models the operation at the level it owns - claim the link,
then post the compensating transfer through the existing `core.Ledger.Transfer` - and
reports the missing seam to the core owner rather than adding it.

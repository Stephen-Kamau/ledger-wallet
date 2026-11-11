<!-- AIGILE-RECORD
kind: plan
stage: plan
round: 1
schema_version: 1.1.0
superseded_by: none
-->

# Plan — burst `architecture-v1`

**Goal.** Produce the ledger-wallet architecture first, through the AI-gile cycle, before
any production code. Backend is Go. Authored by the signed operator roster, exercising the
`core → api → web` dependency chain and the `core ∩ migrations` schema overlap.

**Sequence head.** First burst of a new toy; no prior sequence. Charter Objective:
"Architecture produced first, through the AI-gile cycle, before production code."

**Pre-existing red tests.** NO_BASELINE — fresh repo; `/ag-probe` measures the delta.

## Streams (decomposition)

The dependency chain is deliberate, so the streams run as a short pipeline, not a free
fan-out:

- **S1 core (Amara, architect).** `docs/architecture/00-core.md` — system overview, domain
  model, double-entry invariant engine (R1–R6, R10), the frozen `Ledger`/`Store` interface
  (the chain's seam), concurrency model (R17/R18), and the schema contract (shared with
  migrations). Chain head; runs first and freezes the contract.
- **S2 api (Dakari, builder).** `docs/architecture/10-api.md` — HTTP endpoint design
  (R9–R13), the auth boundary (R7), typed errors. Consumes S1's frozen `Ledger` interface.
- **S3 migrations+tests (Kofi, builder).** `docs/architecture/30-schema-migrations-tests.md`
  — numbered idempotent migrations for S1's schema contract (R14–R16), plus the concurrency
  + throughput test strategy (R17–R19). SHARES the schema contract with S1 (overlap trigger).
  Runs parallel to S2.
- **S4 web (Nia, designer).** `docs/architecture/20-web.md` — balances/statement/transfer UI
  design (R20–R22). Consumes S2's API shape. Runs after S2.
- **S5 review (Zola, runner→reviewer).** Independent review of the assembled architecture for
  soundness and chain coherence. Holds no build stream → review-independence satisfied. Runs
  last.

## Shared files / overlap

- `docs/architecture/` schema-contract section: Amara (S1) authors it; Kofi (S3) consumes and
  implements it. Declared-glob intersection on the schema contract is the intended
  claim-overlap trigger.

## Deliverable

`AIGILE_DESIGN.md` (the canonical "how it's built") assembled from the five sections, plus the
per-stream docs under `docs/architecture/`. This is the binding architecture contract before
any production Go code.

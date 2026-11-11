<!-- AIGILE-RECORD
kind: determination
stage: build
round: 1
schema_version: 1.1.0
superseded_by: none
-->

# Determination — round 1, burst `architecture-v1`

**Outcome.** Architecture produced through the AI-gile cycle by the signed operator roster.
Independent review (Zola, `zola-1`, built none of the streams → review-independence holds):
**APPROVE-WITH-CONDITIONS**.

## What was built

Five architecture docs under `docs/architecture/`, synthesised into `AIGILE_DESIGN.md`:
- 00-core (Amara) — domain model, double-entry invariants, frozen `Ledger`/`Store` seam,
  single-serialized-writer concurrency, schema contract.
- 10-api (Dakari) — endpoints, Bearer auth boundary, typed error→status mapping, idempotency.
- 20-web (Nia) — balances/statement/transfer views, honest error states, int64 money.
- 30-schema-migrations-tests (Kofi) — numbered idempotent migrations, `0002` currency
  metadata, concurrency/throughput test strategy.
- 99-review (Zola) — independent review and verdict.

## Cross-stream events resolved during the burst (the multi-user exercise)

- **GetAccount seam.** Dakari hit a gap in the frozen `Ledger`; routed to Amara (core owner);
  Amara ACCEPTED, added `GetAccount` to the interface. The architect owned the seam change.
- **Schema overlap (core ∩ migrations).** Kofi accepted Amara's idempotency fold, strengthened
  `INDEX→UNIQUE INDEX` on postings, took the append-only trigger, deferred `holds` back to
  Amara. Real cross-operator negotiation on the shared contract.

## Conditions carried to the first build burst (blocking)

- A — own + build `internal/sqlite` adapter and `cmd/ledgerd` (Orchestrator/Amara).
- B — freeze `seq` generation site + ratify UNIQUE index (Amara).
- C — add `GET /accounts` list endpoint + `Ledger` list method for R20 (Dakari + Amara).
- D — decide read-side authorization and 403-vs-404 shape (Dakari + Amara).
- Non-blocking: E (UNIQUE→replay layer), F (link currency scale via `GET /currencies`).

## Disposition

Architecture burst complete. The design is the binding contract for implementation. Conditions
A–D are the agenda for the build burst, not defects in the architecture. No production Go code
written yet, by design (architecture-first).

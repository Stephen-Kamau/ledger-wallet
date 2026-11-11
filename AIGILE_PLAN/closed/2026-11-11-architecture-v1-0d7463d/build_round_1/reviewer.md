<!-- AIGILE-RECORD
kind: reviewer
stage: build
round: 1
schema_version: 1.1.0
superseded_by: none
-->

# Reviewer record — round 1, burst `architecture-v1`

**Reviewer:** Zola (`zola-1`), reviewer seat. Built none of the streams under review →
review-independence holds. Full review: `docs/architecture/99-review.md`.

**Verdict: APPROVE-WITH-CONDITIONS.**

Chain is coherent; single-serialized-writer concurrency (BEGIN IMMEDIATE + in-tx R3 check)
is genuinely sufficient for R17/R18; double-entry invariants (R1–R5) sound.

Adjudications:
- `seq` must be assigned by the adapter inside `InsertTransfer` under the write lock
  (`MAX(seq)+1` per account). Kofi's `UNIQUE(account_id, seq)` strengthening APPROVED.
- `BEGIN IMMEDIATE` + `sql.DB` pooling fine (SQLite serializes writers at file level),
  contingent on `busy_timeout` on every connection.

Blocking conditions (before implementation):
- A — `internal/sqlite` + `cmd/ledgerd` unowned (highest blocker).
- B — freeze seq-generation site + ratify UNIQUE index.
- C — R20 unmet: no `GET /accounts` list endpoint / `Ledger` list method.
- D — read-side authorization undecided (UI codes a 403 the backend never emits).

Non-blocking: E (UNIQUE→replay conversion layer), F (currency scale in two unlinked places).

Consolidated open questions OQ-1..OQ-10 in `99-review.md` §4 with owners.

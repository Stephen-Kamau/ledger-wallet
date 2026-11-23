<!-- AIGILE-RECORD
kind: retrospective
stage: probe
round: none
schema_version: 1.1.0
superseded_by: none
-->

# Retrospective - burst `reversals`, amara stream

## What shipped
`Reverse` on `core.Ledger` as a compensating entry (R1, R2, R3, R4, R5, R7, R13), the
`core.Tx` lookups behind it, and the SQLite implementations. At-most-one-reversal is a
DERIVED primary key, so reversal shipped with no schema change and no second schema source.

## The sweeps
- Corrections queue, WHOLE queue not just this burst's: no `PENDING:` entries anywhere.
- Open backlog entries: AGV-001 verified against code and dated `STILL-TRUE` through
  `ag-backlog verdict`; AGV-002 filed with a stated `defeat: dependency`.

## What was fixed rather than filed
`.aigile/test-suites.tsv` registered only `go.mod,go.sum`, so `ag-test-plan` refused every
real delta and the test-economy rule had no working mechanism behind it. The backlog gate
REFUSED my attempt to file it - "a defect found during a burst is FIXED IN THAT BURST" - and
was right. Fixed and verified: the planner now returns `repo-go` + `repo-bun` for this
burst's delta.

## The lesson worth carrying
The tell is not the duplication - it is that the duplicate sits in the TEST. A test that
restates a constant another layer owns cannot fail for the contract, only for itself, so it
converts a guard into decoration while still showing green. Four of this burst's findings
had that shape. Its sibling: a harness stub that INJECTS a field production never sends makes
every UI pass answer the stub's own question. zola's formulation: "the stub that invents a
field and the test that restates a constant are the same defect - both answer a question
production never asked them."

## What the governance tooling could not do, and what was not done about it
Four gates refused on one cause: this clone's manifest carries empty `builder_declarations`,
`reviews`, `governance_invocations` and `streams`, because an operator clone drives
`ag-phase`/`ag-plan-store` directly as its pack instructs rather than through the
seat-dispatch path those arrays record. Exits 31, 35, 37 and `SHIP_REVIEW_NOT_CONFIGURED`.
Nothing was fabricated to clear any of them; each is recorded in D-A1 with its cause, and
the independence they could not decide was verified in fact by
`ag_claims.check_independence` (zola PASS, all four builders BLOCK).

## Friction worth naming
- A claim's base SHA does not survive a rebase. Four rebases orphaned `911c6a70`; the
  manifest filename names a commit absent from history. kofi hit the same.
- `ag-hav-validate` and `ag-phase archive`'s HAV check read the flat
  `AIGILE_PLAN/current/feedback.md` while the store addresses it burst-scoped, so both warn
  on a record that exists. Same burst-axis mismatch class as AGV-328.
- `ag-rung-report` raises a TypeError with no declared streams.
- Cost is unavailable for an operator clone: no transcript ties the session to the repo.

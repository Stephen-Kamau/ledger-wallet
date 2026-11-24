<!-- AIGILE-RECORD
kind: iteration
stage: plan
round: none
schema_version: 1.1.0
superseded_by: none
-->

# Iteration log - burst `reversals`, amara stream

## Post-probe (2026-11-23)

**Build outcome.** `Reverse(ctx, authorizer, entryID, idempotencyKey)` on `core.Ledger` as a
compensating entry (R1, R2, R3, R4, R5, R7, R13), `FindTransferByID` +
`FindTransferByPostingID` on `core.Tx`, and the SQLite implementations. At-most-one-reversal
is a derived primary key (`rev_<transfer id>`), so no schema change and no new drift surface.

**Probe.** Tests PASS (Go `./... -race` green incl. real R19 at 233s; web 73/73). UI PASS at
1440x900 and 1280x800, four levels each, real ledgerd with the reverse-stub DISABLED. API
PASS 7/7. Scope clean: 9 of 9 code files written are inside the declared globs.

**Reviewer verdict.** APPROVE-WITH-CONDITIONS (zola, 8 rounds). C1-C9 closed; conditions 1
and 2 closed; condition 3 closed upstream by `b6d84ec` after this stream raised the
correction, and verified here by mutation on both failure modes.

**Steer decision.** SHIP ON D-Z1. The Orchestrator declined the `ag-driver profile refreeze`
route and accepted the review as independent on the factual basis recorded in D-Z1.
`ag-gate ship-preflight` will therefore still refuse (no route-bound governance invocation);
that refusal is expected and carries a recorded exception rather than a silent bypass.

**Per-stream final status.** One stream, run inline by the coordinator. STATUS `## Active
streams` is `None.` - no open stream to close.

## Admissions

admission: unsupported-assertion - reported ahead/behind counts against `origin/dev` across several turns while that ref had been stale at 76e9e9f since session start. The arithmetic was internally consistent and wrong; the Orchestrator caught it, not any check of mine.
admission: process-skipped - the web link-rendering fix was written before its test, against `### Tests come first`. Red was proved retroactively by restoring the pre-change module (5 of 7 cases failed), which is evidence after the fact, not red-first.
admission: scope-missed - my own real-adapter test hardcoded `"rev_"` instead of reading the constant, the exact decoration class I later named as this burst's transferable lesson. zola's search missed that fourth site; I had written it.
admission: withdrawn-claim - first ran `check_independence` with a flattened claim shape, so every login returned PASS including my own. Caught by reading the function source before publishing; the corrected run is what the record carries.

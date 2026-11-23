# AIGILE_HISTORY.md

Full Burst archive. Mutable until the NEXT burst ships on top of it. Used for retrospectives and pattern mining.
AIGILE_STATUS.md keeps the rolling last 5 for quick context.
This file keeps everything.

<!-- Entry format:

### [YYYY-MM-DD] Burst: <name>

**Built:** <what was built>
**Verify:** PASS | FAIL
**Steer:** <decision>
**Learnings:** <optional - non-obvious insights, corrections promoted, friction encountered>

-->

### [2026-11-23] Burst: reversals (amara stream — core/**, internal/sqlite/**)
**Built:** `Reverse(ctx, authorizer, entryID, idempotencyKey)` on `core.Ledger` — a compensating entry with mirrored postings (R1, R2, R5), R3/R7 enforced on the account the reversal DEBITS, R4 replay, R13 typed conflicts. `FindTransferByID` and `FindTransferByPostingID` on `core.Tx` plus their SQLite implementations. At-most-one-reversal is a DERIVED primary key (`rev_<transfer id>`), so reversal shipped with no schema change and no second schema source to drift.
**Verify:** PASS — Go `./... -race` green across all packages including the real R19 (233s); web 73/73; UI at 1440x900 and 1280x800, four levels each, against real ledgerd with the reverse-stub DISABLED; API 7/7 including 201 to a POSTING id, 409 `already_reversed`, 404 `entry_not_found`. Scope clean: 9 of 9 code files written are inside the declared globs.
**Steer:** SHIP on D-Z1. Reviewer verdict APPROVE-WITH-CONDITIONS (zola, 8 rounds); C1–C9 closed, conditions 1–3 all closed by ship. `ship-preflight` still refuses (PROFILE_STALE, no route_token); shipped over the recorded exception D-A1 rather than refreezing governance routing to buy a green gate.
**Learnings:**
- **The tell is not the duplication — it is that the duplicate sits in the TEST.** A test restating a constant another layer owns cannot fail for the contract, only for itself: a guard turned into decoration that still reports green. True of all four of this burst's silent cross-layer findings. Closed by `tests/reversal_contract_test.go`, which fails closed when it can no longer see its subject.
- **A harness stub that injects a field production never sends makes every UI pass answer the stub's own question.** The live statement emits neither `reverses_entry_id` nor `reversed_by_entry_id`; the stub supplied both, so every browser pass before the endpoint landed was hollow. Found only because the Orchestrator's instruction required a page RELOAD.
- **Three green suites can hide a seam defect none of them can see.** `entryID` was looked up as a transfer id while the UI sends a posting id — core tested with transfer ids, api with a configured fake, web with a stub. Found by a live e2e, not a unit suite.
- **A claim's base SHA does not survive a rebase.** Four rebases orphaned `911c6a70`; the manifest filename now names a commit absent from history. kofi hit the same. Not a mistake by either of us — a property of the claim mechanism under concurrent operators.

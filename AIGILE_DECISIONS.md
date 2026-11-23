# AIGILE_DECISIONS.md

Record of challenged/resolved design decisions. Prevents relitigating settled calls.

## build-v1 — resolving the architecture review's conditions (coordinator rulings)

These resolve the four blocking conditions from the `architecture-v1` independent review
(`docs/architecture/99-review.md`). Additive, in-scope, no CHARTER Hard Spec touched — made by
the burst coordinator and recorded here.

- **D-A1 [2026-11-11] (condition A) — `internal/sqlite` and `cmd/ledgerd` ownership.**
  Assigned to **Amara** (she owns the `Store` port/contract, so the adapter that implements it
  and the server entrypoint that wires it belong with core). Ruling: these packages are Amara's
  stream scope in `build-v1`.
  *Note: this is deliberately the overlap surface for the multi-user exercise — Dakari needs a
  running server for his API and will reach for `cmd/ledgerd`; that grab is expected to trip the
  overlap/ownership gate against Amara's claim.*

- **D-B1 [2026-11-11] (condition B) — `seq` generation + UNIQUE index.** `postings.seq` is
  assigned by the SQLite adapter inside `InsertTransfer`, under the `BEGIN IMMEDIATE` write lock
  (`MAX(seq)+1` per `account_id`). Kofi's `UNIQUE(account_id, seq)` strengthening is ratified.
  Confirms Zola's adjudication.

- **D-C1 [2026-11-11] (condition C) — R20 "accounts" plural.** Add
  `AccountsByOwner(ctx, PrincipalID) ([]Account, error)` to the `Ledger` interface and a
  `GET /accounts` endpoint (owner = authenticated bearer principal). Purely additive to the
  frozen contract; Amara owns the interface edit, Dakari the endpoint.

- **D-D1 [2026-11-11] (condition D) — read-side authorization.** Balance and statement reads
  require the authenticated principal to **own** the account. A read of an account the principal
  does not own returns **404** (not 403) to avoid leaking account existence. System/mint
  accounts are readable only by the configured admin set. The web layer drops its "403 not your
  account" state accordingly (reads are 404 or 200).

## build-v1 — independent review findings (Zola, `docs/operators-review-build-v1.md`)

Verdict APPROVE-WITH-CONDITIONS. Dispositions (coordinator):

- **C-1 (medium) — FIXED in burst.** Adapter computed `postings.seq` but never wrote it back into
  the returned transfer result (real adapter reported `seq:0`; fakes hid it). A defect found in a
  burst is fixed in the burst — Amara fixed it with a real-adapter test the fakes cannot satisfy.
- **C-2 (medium) — RECONCILED (docs).** R19 baseline contradicted the "reads dwarf writes" design
  claim. Not a code bug; `AIGILE_DESIGN.md` concurrency section now records the empirical reality.
- **C-3 (low) — DEFERRED.** UNIQUE→replay backstop not implemented in `InsertTransfer` (returns a
  raw error on constraint violation). Near-unreachable under `BEGIN IMMEDIATE` (the in-tx
  `FindTransferByKey` fast-path handles replay). **Trigger:** if the serialized-writer model is
  ever relaxed (per-account locks / optimistic concurrency), implement the backstop conversion and
  a test that reaches it.
- **Conflict resolution — VERIFIED sound.** Zola confirmed via `go list` that no `api→cmd` or
  `core→cmd` edge exists; dependency direction `cmd→{api,core}` intact after Amara's revert.

## reversals — reviewer independence: EXCEPTION REQUESTED (Zola)

- **D-Z1 [2026-11-23] — `review-independence` cannot be satisfied; exception requested, NOT
  self-certified. → GRANTED by the Orchestrator, 2026-11-23, on the factual basis below.** `ag-gate review-independence` exits **37**:
  `no signed reviewer identity on any verdict - review-independence cannot be established;
  record the review under an enrolled operator`. There is no compliant route to satisfy it in
  this repo: `ag-role-review status` reports **`roles not configured`**, so no signed reviewer
  verdict can be recorded at all. (Earlier in the burst the same gate exited **21** for want of
  a burst base; that was a precondition, not a verdict.)

  **The property holds in fact.** I hold no claim — I minted none, by design, since a
  reviewer's independence is the absence of one. I wrote only `docs/` (`operators-review-reversals.md`,
  `operators-activity-log.md`) plus this entry, and no file in any builder's claimed scope:
  nia `web/**`, kofi `migrations/` + `tests/`, amara `core/` + `cmd/`, dakari `api/`.

  **It is not attested.** The machinery that would prove it cannot run, and I declined to
  self-certify — a reviewer vouching for their own independence is the exact thing the gate
  exists to prevent, so asserting it myself would defeat its purpose rather than satisfy it.
  Per `### Stage ownership and gate discipline`, a gate with no compliant route needs an
  explicit, recorded Orchestrator exception. **This entry is that request.**

  **DECISION — GRANTED [2026-11-23].** The Orchestrator accepted the burst `reversals` review as
  independent on the factual basis above: *"accepted on the factual basis — grant D-Z1"*. The
  burst's review verdict (APPROVE-WITH-CONDITIONS) stands as an independent review.

  **What this grant does NOT do, and should not be read as doing:**

  1. **It does not fix the machinery.** `ag-role-review status` still reports `roles not
     configured`, so `ag-gate review-independence` will still exit **37** on the next burst.
     This is a **per-burst exception for `reversals`**, not a standing waiver. The next burst
     either configures roles or needs its own recorded exception — it does not inherit this one.
  2. **It does not make the gate's PASS available.** Independence here rests on a recorded
     factual basis (no claim held, `docs/` only, no builder scope touched) that a human accepted,
     not on an attestation anything can re-check later. An auditor reading this repo in six
     months has this entry and the activity log, not a gate result.
  3. **It does not cover the one out-of-scope write.** `core/id_test.go` was written by the
     reviewer in amara's scope on explicit Orchestrator instruction, after the builders stood
     down, and is recorded as such in `docs/operators-activity-log.md`. It is disclosed, not
     laundered by this grant.

  **Standing recommendation:** configure `ag-role-review` before the next burst, so independence
  is attested rather than accepted. One exception is a judgement; a pattern of them is an
  unenforced control, and `### A control must earn its keep` would then apply to the gate itself.

  **Related and distinct — stale claims, confirmed independently.** 6 live claims, all from
  shipped bursts (`architecture-v1`, `build-v1` x4, `collision-v2`), all still `"status":
  "in-flight"` with `"finalised_at": null`. They produce a **false-positive** overlap WARN
  against a shipped `build-v1` claim. The Orchestrator reproduced this in kofi's clone
  independently — 5 WARNs, 4 of them dead claims from shipped bursts, one kofi's own — and filed
  it as **AGV-396** in canonical. A guard that always warns is a guard nobody reads; the fix is
  to finalise claims at ship and sweep the existing six. Tracked in canonical, not here.

---

## D-A1 — Ship `reversals` (amara stream) on D-Z1, without refreezing the Reviewer profile

**Date:** 2026-11-23 · **Decided by:** Orchestrator ("ship it on D-Z1") · **Scope:** this burst only

**The gate that refuses.** `ag-gate ship-preflight` exits 31 for this burst: the manifest
records no route-bound governance invocation. It cannot be made to, because
`ag-resolve-role --json Reviewer` returns `state: PROFILE_STALE` (profile `codex-opencode`)
with **no `route_token` field at all**, and `record-governance-invocation` requires one.

**Options put to the Orchestrator, and the one taken.**

- *Refreeze* — `ag-driver profile refreeze Reviewer` records a new route in the manifest and
  the gate then passes. **Declined.** The staleness follows the canonical install moving
  16.21 → 16.23 during this session, not an intended seat edit, so refreezing would make the
  manifest name a route that never ran — buying a green gate with a false record.
- *Ship on D-Z1* — **taken.** Independence was already accepted on a recorded factual basis
  in D-Z1, and the property the gate protects is verified true rather than attested:
  `ag_claims.check_independence` over every claim event in this clone returns **zola PASS**
  (holds no claim) and **BLOCK for all four builders** (amara, dakari, kofi, nia). zola is
  the only eligible reviewer of this burst and reviewed it across 8 rounds.

**What this exception does NOT do.** It does not make `ship-preflight` pass — the refusal
stands and `/ag-ship` proceeds over a recorded exception, never a silent bypass. It does not
resolve the Reviewer seat, so the next burst inherits `PROFILE_STALE` and must decide again.
It is scoped to this burst, exactly as D-Z1 was.

**EXTENDED 2026-11-23 to cover all three ship gates.** The steer that authorised D-A1 named
only exit 31, because only exit 31 had been run at that point. Running the universal
pre-flights surfaced two more refusals, both put to the Orchestrator before proceeding and
both granted for this burst:

```
SHIP_PREFLIGHT_EXIT: 31          no cleared build-stage review verdict        (D-A1, original)
INDEPENDENCE_PREFLIGHT_EXIT: 35  Builder declaration identity absent/malformed (D-A1, extended)
REVIEW_INDEPENDENCE_PREFLIGHT_EXIT: 37  no signed reviewer identity on a verdict (D-Z1 named it)
```

All three have ONE cause, visible in the burst manifest: `builder_declarations`,
`builder_terminal_invocations`, `reviews`, `governance_invocations` and `streams` are all
EMPTY. This clone drove `ag-phase` and `ag-plan-store` directly, exactly as the operator
pack instructs, rather than through the seat-dispatch path that populates those arrays. So
independence is UNDECIDABLE to the gates while being demonstrably TRUE in fact:
`ag_claims.check_independence` returns zola PASS (holds no claim) and BLOCK for all four
builders.

**A fourth instance, same cause, recorded at the close-out.** `ag-ship-review-dev` returned
`SHIP_REVIEW_NOT_CONFIGURED - could not resolve a Ship-review seat and no --model given`.
The repo's `ship_review_policy` is undeclared and therefore `blocking`, under which only
`SHIP_REVIEW_DONE` proceeds. The SUBSTANCE of that gate is satisfied and recorded: zola's
ship-level reviewer sign-off (`docs/operators-review-reversals.md`, "Burst `reversals` -
reviewer sign-off") is FINAL under D-Z1, verdict APPROVE-WITH-CONDITIONS, with conditions 1
and 2 closed there and condition 3 closed afterwards by `b6d84ec` and mutation-verified by
this stream on both failure modes. What cannot be produced is the seat token, for the same
reason as the other three.

**What was explicitly NOT done.** No `builder_declarations` entry was fabricated to turn
exit 35 green. That is the same trade declined on the refreeze: a green gate bought with a
record of something that did not happen is worse than a refusal that is written down.

**Scope of the extension.** This burst, DEV FLOW close-out only. `dev` -> `main` promotion is
NOT covered and was deliberately left to the coordinator, once all five operator streams are
in. A five-operator burst's promotion is not one operator's call.

**Standing recommendation.** Resolve the profile staleness while Idle (`ag-init` reselect),
not mid-burst, and make `ag-operator join` populate a builder declaration so an operator
clone's manifest can satisfy the gates it is measured by. THREE accepted-on-facts exceptions
across one burst (D-Z1 for 37, D-A1 for 31 and now 35) is the pattern
`### A control must earn its keep` says to look at: these controls are not enforcing on this
path, they are being routed around on judgement each time. Either the operator-clone flow
should populate what they read, or the gates should state that they do not apply to it.

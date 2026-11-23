# Operator review - burst `reversals`

**Reviewer:** zola (Zola Juma), runner rung, holds no build claim.
**Reviewed at:** `946b5fa` (dev), 2026-11-23 UTC.
**Streams landed at time of review:** nia only (`964ce8c`, `a7a9205`, `946b5fa`, plan `5977f41`).

## Verdict: APPROVE-WITH-CONDITIONS

nia's stream is in scope, well-built and green. Two conditions below are defects in
landed code; the third is a burst-level gate that cannot be closed until dakari lands.

---

## Evidence

| Check | Result |
|---|---|
| `go build ./...` / `go vet ./...` | clean |
| `go test ./... -race` (full suite) | **green**, exit 0 (`tests` 575s) |
| `bun run test` (web) | **green**, 52 tests / 4 files |
| Live e2e vs booted `ledgerd` | **all pass**, incl. conservation |
| Merge conflict | none - rebase applied clean, nothing overwritten |
| Scope vs claim | no out-of-scope code write |

**Live e2e** (real `ledgerd`, temp DB, `-admins treasury`): account creation; non-admin
`allow_negative` -> 403 `forbidden_privileged_flag`; mint 10000; transfer 2500; idempotent
replay (`replayed=true`); cross-principal debit -> 403 `unauthorized`; balances 7500/2500;
**conservation: sum of all postings = 0** (treasury -10000, alice +7500, bob +2500).

**Backend is byte-identical to the pre-burst baseline.** `git diff --name-only 76e9e9f..HEAD
-- '*.go' '*.sql' go.mod go.sum` is empty. nia changed no Go.

---

## Scope review

nia's declared scope is `web/**`, recorded **and signed** in her claim event
(`.aigile/manifests/5977f419....events.jsonl`, `kind: claim`, `"globs": ["web/**"]`) and
matching her round plan. Files written outside it: her own plan record, her own signed manifest,
`AIGILE_STATUS.md` (phase transition), and `docs/operators-activity-log.md` (required by
the operator pack). All governance or instructed. **No out-of-scope code write.**

---

## Conditions

### C1 - BLOCKING. Reversal idempotency key is minted per CALL, not per intent; a retry can double-post.

`web/src/views/StatementPane.tsx:129` calls `client.reverseEntry(entryId, newIdempotencyKey())`,
minting a **fresh** key on every invocation. The comment two lines above it claims the
opposite:

> "One idempotency key per reversal INTENT (R4): a retried or double-submitted reverse
> returns the original correction instead of posting a second one."

The code does not have that property. The retry path is live: `reversalErrors.ts`
`mapReversalError` returns `action: "retry"` for the generic bucket, and
`StatementPane.tsx:334-335` renders that as a button wired to `onConfirm` -> `reverse(entryId)`
-> a new key. Against a backend that enforces R4 idempotency **by key**, the second call is a
distinct intent and posts a second compensating transfer. Money moves twice.

It is masked today only because nia's own stub guards on `reversedBy.has(entryId)`
(`web/harness/reverse-stub.mjs:117`) and returns 409 regardless of key. The protection
is the server's already-reversed check, not the key - so the comment is wrong in either case.

**Fix:** mint one key per entry per reversal intent and reuse it across retries (e.g. hold it
in `reversing`/row state), or state explicitly that the server guards by entry and correct the
comment. The unit suite does not cover this - 52 tests pass with the defect present.

### C2 - BLOCKING. Ambiguous failures assert "was not reversed", which can be false.

Two paths state a financial fact the client cannot know:

- `web/src/views/reversalErrors.ts:152-157` (generic bucket): *"This entry was not reversed."*
- `web/src/views/StatementPane.tsx:143-147` (network failure): *"Could not reach the server.
  This entry was not reversed."*

`internal` is deliberately routed to the generic bucket, and `api/errors.go:53-59` shows
`writeJSON` emits `500 internal` when the **success** body fails to marshal - i.e. **after the
ledger has committed**. A panic after commit funnels to the same place. A network failure
after the request reached the server is equally ambiguous. In all three the reversal may have
posted while the user is told it did not - and is then offered a retry (see C1).

**Fix:** for unknown/`internal`/network, say the outcome is unconfirmed and send the user to
refresh, not retry.

### C3 - NON-BLOCKING, burst gate. The mandatory live reversal e2e cannot be run.

The operator pack requires a live end-to-end that reverses, replays the reversal idempotently,
reverses an already-reversed entry (409), and debits another principal's account (403).
**`POST /entries/{id}/reverse` does not exist on dev** - no route in `api/handler.go`, nothing
in `core/`. nia built against `web/harness/reverse-stub.mjs`, which is correct practice for a
parallel stream, but the stub is **self-confirming**: it returns `already_reversed` and
`unauthorized` because those are the same spellings nia guessed in
`REVERSAL_CODE_ALIASES`. It is not evidence about dakari's endpoint.

This is stated, not worked around. The gate closes when dakari lands; the reversal legs of my
e2e are written and skip cleanly until then.

### C4 - ADVISORY. Delete the alias hedge when the real contract lands.

`REVERSAL_CODE_ALIASES` (`reversalErrors.ts:40-45`) folds `entry_already_reversed` /
`posting_already_reversed` onto `already_reversed`. nia's own comment says to delete it once
the endpoint lands. Holding her to it: if dakari ships a spelling not in that list, the UI
degrades silently to the generic bucket - which, per C2, currently lies about the outcome.

---

## Governance finding - stale claims are never finalised

**Correction to an earlier draft of this review:** I first recorded that claim manifests
carry no scope, on the basis of `"streams": []` in the `.json`. That was wrong. Claim globs
are recorded and signed in the `.events.jsonl` claim event (`"globs": ["web/**"]`);
`streams` is a different field. Scope review from the manifest **is** possible, and I used it.

The real defect is that **claims are never finalised**. `ops ag-plan-overlap` reports 6 live
claims; all six are from prior bursts (`architecture-v1`, `build-v1` x4, `collision-v2`),
every one still `"status": "in-flight"` with `"finalised_at": null`, despite those bursts
having shipped (`collision-v2`'s own manifest records phases through `Promoting`).

This is not cosmetic. It produces a **false positive** on the live gate:

```
WARN: planned scope overlaps live burst 2b7a0366... (owner nia) on ['(declared-glob overlap)']
ag-plan-overlap: 1 overlap(s) WARNED; the plan-time guard never blocks
```

`2b7a0366` is a **build-v1** claim that shipped. The overlap machinery works correctly - it
is being fed dead claims. Every burst from here will accrue another spurious WARN, and a
guard that always warns is a guard nobody reads.

**Recommendation:** finalise claims at ship, and sweep the 6 stale ones.

### `review-independence` cannot pass as this repo is configured

The gate now exits **37**:

> `AG_GATE_ERROR: review independence: no signed reviewer identity on any verdict -
> review-independence cannot be established; record the review under an enrolled operator`

But `ag-role-review status` reports **`roles not configured`**, so there is no configured
route to record a signed reviewer verdict in this repo. The gate cannot be satisfied.

Stating it rather than working around it, per the operator pack. The independence property
holds in **fact** - I hold no claim, wrote no `web/` file, and my scope is `docs/` only - but
it is not currently *attested* by the machinery. I cannot self-certify that; it needs roles
configured, or an Orchestrator exception recorded in `AIGILE_DECISIONS.md`.

## Merge integrity

One rebase onto `origin/dev`, clean, **no conflict and no overwrite**. nia's 17 files applied
whole. Nothing of another operator's was discarded; no other operator had landed at that point.
My own working tree carried only provisioning noise (`CLAUDE.md`, `BACKLOG.md`,
`.claude/.aigile-version`) plus `tests/throughput_baseline.txt`, which the throughput test
rewrites as a side effect - all autostashed and none committed.

---

# Review round 2 - kofi's stream (`migrations/`, `tests/`)

**Reviewed at:** `3a9e6ef` (dev), 2026-11-23 UTC. Streams landed: nia, kofi. **dakari still absent.**

## Verdict on kofi's stream: APPROVE

| Check | Result |
|---|---|
| `go build ./...` / `go vet ./...` | clean |
| `go test ./... -race` (full suite) | **green**, exit 0 (`tests` 345s) |
| Scope vs signed globs `['migrations/', 'tests/']` | clean - wrote exactly that |
| R5 append-only triggers | **intact** - `postings_no_update` / `postings_no_delete` unchanged, no `DROP` anywhere |
| `0004_reversals.sql` idempotency | every `CREATE` guarded `IF NOT EXISTS` |
| Merge integrity | clean rebase; zola/nia/kofi activity rows all present, nothing overwritten |

`0004_reversals.sql` is rigorous: `UNIQUE (original_transfer_id)` plus settle-once,
no-repoint and no-delete-settled triggers. `TestR18_ConcurrentReversalsOfSameEntry_ExactlyOne`
races 32 goroutines with **deliberately distinct idempotency keys** and asserts exactly one
claim wins; kofi verified it fails correctly with the constraint removed.

Two edits to existing tests were checked for weakening and are **improvements**:
`migrations_test.go` now walks `migrations.Names()` instead of a hardcoded filename list, so
a new migration cannot silently escape parity; the R19 `-short` skip carries a stated, measured
precondition and the default full run still measures it.

## C1 - DOWNGRADED

My round-1 C1 said a retried reversal could post a second correction and move money twice.
**kofi's `UNIQUE (original_transfer_id)` makes that structurally impossible**, independent of
the idempotency key - which is exactly what `TestR18` proves with distinct keys.

What survives is narrower: nia's comment at `StatementPane.tsx:129` credits the idempotency
key for a guarantee the key does not provide (the database constraint does). The comment should
name the real mechanism. **No longer blocking.**

## C5 - BLOCKING, CROSS-STREAM. A failed compensating transfer bricks reversal for that entry forever.

Neither operator can see this alone; it exists only in the seam between them.

`core` has **no `Reverse` and no `Tx.InsertReversal`** (verified: `grep -rn 'func.*Reverse\|InsertReversal' core/ internal/sqlite/` is empty). kofi reported this to amara and it was not taken. So reversal is claim-then-settle across **two** transactions, and `tests/reversals_test.go` `reverse()` shows the consequence:

1. `claimReversal` INSERTs the claim and **commits**.
2. The compensating transfer runs. If it fails, the helper `return true, terr` - **the claim row is left behind, unsettled.**
3. `UNIQUE (original_transfer_id)` now rejects every future attempt to reverse that entry.

The failure in step 2 is not exotic. `insufficient_funds` on the compensating leg is the
ordinary case where the destination has already spent the money - and it is a case nia wrote
UI copy for.

kofi designed the escape hatch: the schema deliberately permits deleting an *unsettled* claim,
asserted at `tests/reversals_test.go:401-405` ("rollback path when the compensating transfer
fails after the claim commits"). **Nothing performs that rollback**, and there is no `core`
seam to own it. A crash between commit and settle orphans the claim with no cleanup at all.

**The cross-stream contradiction:** nia's `insufficient_funds` copy reads *"The entry was not
reversed."* - which the user reasonably reads as "try again later". After the first attempt
they never can. The UI promises retriability the schema has permanently revoked.

**Fix:** give `core` a `Reverse` seam that writes the claim and the compensating postings in
one transaction, or make the caller delete the unsettled claim on settle failure and add a
sweep for orphans. Until then C2's copy is actively misleading on this path.

## C3 - still open

`POST /entries/{id}/reverse` **still does not exist**. The mandatory live reversal e2e
(reverse / idempotent replay / 409 / 403) remains unrunnable. The burst is not shippable.

## Burst-level verdict: REQUEST_CHANGES

Not against either landed stream - both are good work and both are APPROVEd on their own
scope. Against the **burst**: C5 is a money-path liveness defect in the seam, and C3 means the
one verification that would have caught it end to end has never been run.

---

# Review round 3 - nia's fixes to C1, C2 and the C5 web half

**Reviewed at:** `00caf7a` (dev), 2026-11-23 UTC. **dakari still absent.**

Re-reviewed because C2 and C5 were blocking; C1 was re-checked with them as it shares the
same call path. All three verified against the code, not the commit message.

| Finding | Status |
|---|---|
| C1 idempotency key per call | **RESOLVED** |
| C2 false "was not reversed" | **RESOLVED** |
| C5 web half (promised retry) | **RESOLVED** |
| C5 backend seam | **STILL OPEN - blocking** |
| C3 live reversal e2e | **STILL OPEN - blocking** |
| C4 alias hedge | still present, correctly (dakari has not landed) |

**Evidence:** `bun run test` **62 passed / 5 files** (was 52/4; +10 covering the fixes).
Backend untouched since the green `-race` run at `3a9e6ef` - `git diff --name-only
ce76775..HEAD -- '*.go' '*.sql' go.mod go.sum` is empty - so that run stands as the backend
evidence rather than a re-run. Activity rows intact: zola 18 / nia 14 / kofi 22.

**C1.** `ReversalIntents.keyFor(entryId)` (`web/src/views/reversalIntent.ts`) memoises one key
per entry, so a retry reuses it. The lifecycle is right where it matters: the intent is
**closed on confirmed success** (`StatementPane.tsx:149`) and on explicit cancel/dismiss
(`:230`, `:236`), **cleared on refresh** (`:107`), and deliberately **left OPEN on a transport
failure** (`:155-158`) so a retry cannot mint a second key. That last case is the one that
mattered.

**C2.** `unconfirmedOutcome()` replaces the generic bucket: *"We cannot confirm whether this
entry was reversed — refresh the statement to see what actually posted."* with
`action: "refresh"`, not `retry`. The claim the client is not entitled to make is gone, and
the unsafe next move with it.

**C5 web half.** The `insufficient_funds` copy now names the dead end rather than implying a
later retry: *"No money moved. This entry cannot be reversed again — post a correcting
transfer instead."* Correctly scoped - nia notes in the file that the seam fix is core/api
work outside her stream, which is right.

## Burst verdict: REQUEST_CHANGES (unchanged)

Both web findings are closed and closed properly. The burst still cannot ship:

- **C5 backend seam.** `core` still has no `Reverse` / `Tx.InsertReversal` (re-verified: the
  grep over `core/ internal/sqlite/ api/` is empty). A failed compensating transfer still
  strands the claim, and `UNIQUE (original_transfer_id)` still bricks that entry permanently.
  nia's copy now tells the truth about that dead end - it does not remove it.
- **C3.** `POST /entries/{id}/reverse` still does not exist. The mandatory live e2e has
  still never run.

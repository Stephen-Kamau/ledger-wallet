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

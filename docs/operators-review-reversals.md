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

**Raised as a formal exception request: see `AIGILE_DECISIONS.md` D-Z1 [2026-11-23].** The gate
later exits **37** (`no signed reviewer identity on any verdict`) once a burst base exists, and
`ag-role-review status` reports `roles not configured` - so there is no compliant route at all.
D-Z1 states the factual basis for independence and puts the decision to the Orchestrator rather
than assuming it.

**Stale claims: confirmed independently and filed.** The Orchestrator reproduced this finding in
kofi's clone - 5 WARNs, 4 of them dead claims from shipped bursts, one kofi's own - and filed it
as **AGV-396** in canonical. Tracked there, not here.

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

---

# Review round 4 - amara's `core.Reverse`, kofi's C5 response

**Reviewed at:** `37c66fa` (dev), 2026-11-23 UTC. Streams landed: nia, kofi, amara.
**dakari: claim and plan only, no code.**

| Check | Result |
|---|---|
| `go build ./...` / `go vet ./...` | clean |
| `go test ./... -race` (full suite) | **green**, exit 0 (`tests` 449s) |
| `bun run test` (web) | **green**, 65/65 |
| Live e2e vs booted `ledgerd` | **all pass**, conservation holds after amara's core refactor |
| `gofmt -l` | **`api/fakes_test.go` unformatted** |
| Merge integrity | clean rebase; zola/nia/kofi/amara/dakari rows all intact |

## C5 - RESOLVED in production

amara's `core.Reverse` derives the compensating entry's id as `rev_<entryID>`, so "at most one
reversal per entry" falls out of the `transfers` PRIMARY KEY **inside a single write
transaction**. I verified the single-transaction claim rather than accepting it:
`grep -c InTx core/reversal.go` is **1**. There is no claim row and therefore nothing to
strand - the defect is designed out, not patched. A failed compensation rolls back whole and
the entry stays reversible.

kofi also closed the caller half in the test helper (`releaseClaim`, released when the
compensating transfer did not post, **kept** when it posted but the link failed to settle -
correctly asymmetric), and is explicit that a crash between claim commit and release still
orphans a claim with no sweep. That residual is a property of the **test helper**, not of the
running system, and kofi says so. Accepted as closed.

Good cross-stream behaviour worth recording: amara renamed the not-found code to
`entry_not_found` to match nia's already-shipped UI map, rather than making nia chase it.

## C6 - NEW, BLOCKING. The required typed 409 will be emitted as a 500.

`core/errors.go` defines `ErrAlreadyReversed` (`already_reversed`) and `ErrEntryNotFound`
(`entry_not_found`). **Neither appears in `api/errors.go`'s `coreErrorMap`** - verified by
grep. `writeCoreErr` (`api/errors.go:91-100`) falls through to
`500 internal` for any unmapped core error.

So the moment a handler calls `core.Reverse`:

- a double-reverse returns **500**, not the **409** the burst brief requires;
- an unknown entry returns **500**, not **404**.

It compounds across streams: nia's UI routes `internal` to `unconfirmedOutcome` - *"We cannot
confirm whether this entry was reversed"* - which is maximum uncertainty for the one case the
ledger knows with certainty. The honest 409 copy nia already wrote
(`already_reversed`: "the correction is already on this statement") would never be reached.

Latent today because no handler exists. **Raised now because the fix is two rows in
`coreErrorMap`, and it is cheapest before dakari writes the handler rather than after.**

## C7 - NEW, minor. `api/fakes_test.go` fails gofmt.

`19c2083` (amara) added `Reverse` to `fakeLedger` and left the struct field block
misaligned. Repo standard is gofmt. One command.

## C3 - still open, still blocking

**There is no HTTP route.** `POST /entries/{id}/reverse` does not exist anywhere in `api/` or
`cmd/`. `core.Reverse` is complete and tested; nothing exposes it. nia's UI 404s, and the
mandatory live reversal e2e (reverse / idempotent replay / 409 / 403) **has still never run**.

## Burst verdict: REQUEST_CHANGES

Every landed stream passes on its own scope. The burst does not ship: C3 leaves the
reversal path unreachable and unverified end to end, and C6 means the typed 409 the brief
requires is wired to produce a 500 the moment the path opens.

---

# Review round 5 - dakari's handler; the seam defect, and its root cause

**Reviewed at:** `4807855` (dev), 2026-11-23 UTC. All five streams landed.

Everything below was re-derived here. dakari reported the defect and then corrected
themselves twice; both the report and both corrections were verified against the code before
being accepted, and one of them changes my recommendation.

## Closed this round

| Finding | Status | Evidence |
|---|---|---|
| **C3** live reversal e2e | **CLOSED AS WRITTEN** | route exists; the e2e has now run |
| **C6** unmapped 409/404 | **RESOLVED** | `coreErrorMap` rows 49-50, plus a guard test |
| **C7** gofmt | **RESOLVED** | `gofmt -l` clean |
| **C1** fresh-key double-post | **EMPIRICALLY CLOSED** | fresh key returns 409, not a second posting |

dakari's sharper reframing of C3 is accepted: C3 as written was "no route, e2e never run", and
both are now false. What follows is a **new** finding the e2e produced, not C3 surviving.

C6's fix is better than the two rows I asked for: `TestErrorMap_EveryCoreSentinelMapped`
(`api/api_test.go:390`) now asserts every exported `core.Err*` sentinel has a row, so the next
unmapped sentinel is a failing test rather than a silent 500. That is the class fixed, not the
instance.

**My own live e2e, six legs, against a real `ledgerd` + real SQLite:**

```
201  reverse, authorised by the DESTINATION owner
200  idempotent replay, same key, replayed=true, same transfer id
409  already_reversed, FRESH key        <- C1's exact path
403  unauthorized, source owner attempts it
400  idempotency_key_required
401  no bearer
     CONSERVATION after reversal = 0 (treasury -10000, alice 9900, bob 100)
```

## C8 - RAISED AND CLOSED THIS ROUND. (Was: the real UI cannot reverse anything.)

Independently reproduced here, not taken on report:

```
posting id   post_000001a208f434f9fbcdb62942041319  -> 404 entry_not_found
transfer id  xfer_000001a208f434f97bf5972ba8e35d81  -> 201
```

`core.Reverse`'s `entryID` is looked up as a transfer (`core/reversal.go:55`,
`tx.FindTransferByID`). The UI sends `p.id` (`StatementPane.tsx:233`), the posting id - and
`postingResp` exposes `id` and `transfer_id` as distinct fields, so the row genuinely carries
both. **Every reversal from the shipped UI 404s.**

**This is not a bug in the web stream.** See the root cause below: a statement row IS a leg,
and `core.Entry` is defined as a leg, so sending the row's own id is what this repo's own
defined term implies. The defect is the term, and it is in the seam. Attributing it to the
call site would be wrong.

Confirmed that no stream's tests can reach it, which is the part worth keeping: dakari's
handler tests drive a `fakeLedger` the test configures, so the id kind never matters; nia's
drove her stub; amara's core tests pass transfer ids to themselves. Each is self-confirming in
exactly the way I flagged about the stub in round 1. **`grep` for any test driving a posting id
into reverse returns nothing.** The defect exists only in the seam, and only a live e2e
crosses it.

## Root cause: the burst misuses a term this repo has already defined

This is not loose wording, and it is sharper than "entry means three things".
**`core.Entry` is a defined type in the same package**, and the spec is explicit:

> `docs/architecture/00-core.md:106` - *"Entry is an INTENDED **leg** of a transfer - the
> request-level unit. A transfer is a set of entries that must net to zero (R1)."*
> (`core/posting.go:9`)

So in this codebase an Entry **is a leg**. Which means:

- `core.Reverse(entryID)` accepting **only** a transfer id contradicted a type defined beside
  it. That was the defect, and it was core's, not the route's.
- `POST /entries/{id}/reverse` is the **right** name - an entry is a leg, so the route should
  take a leg's id. It did not, until `321398d`. The name was never the problem.
- **nia's UI was the spec-consistent reading.** A statement row is a leg; sending the row's own
  id is what the defined term implies. The UI is the one component that did not drift.

I did not follow my own finding to that conclusion on first pass - I reached for a route rename
when the definition pointed at the parameter. See the withdrawal below.

## Recommendation: WITHDRAWN. amara's shipped fix is right; mine was not.

I recommended the rename (`POST /transfers/{id}/reverse`, `entryID` -> `transferID`, UI sends
`p.transfer_id`). **I am withdrawing that.** dakari challenged it, asked me to check two things,
and both check out - against my own recommendation.

**It contradicted the finding I used to justify it.** I argued from `00-core.md:106` that an
Entry IS a leg, that a statement row is a leg, and that the UI sending the row's own id was
therefore the spec-consistent reading. Option 4 then has the UI *stop* sending the row's own id.
If an entry is a leg, a route called `/entries/{id}` **should** take a leg's id - which is
exactly what amara's `resolveEntry` now makes it do. The defect was never that the route said
"entries"; it was that core accepted only transfer ids under a parameter named `entryID`.
amara fixed precisely that mismatch.

**And the cost comparison I gave was inverted.** I claimed option 2/4 would silently break
nia's overlay. Verified empirically at `321398d`: `api/types.go:112` echoes `ReversesEntryID:
entryID` - **the id the client sent**. Driving the posting id through a real server:

```
POST /entries/post_000001a208fd9837a44b104d8c02b88d/reverse  -> 201
     reverses_entry_id = post_000001a208fd9837a44b104d8c02b88d   (the posting id, echoed)
POST /entries/xfer_.../reverse  (same entry, other id, FRESH key)  -> 409 already_reversed
     CONSERVATION = 0
```

So `reversedEntryId` is a posting id, `reversalIndex.ts:67` compares posting id to posting id,
and **the overlay works unchanged. amara's fix costs the web stream nothing.** Option 4 is now
the *only* option that would require the overlay rework - the very silent-failure risk I
promoted to its own section as the most important finding in this burst. I would have been
mandating the risky change to buy a name less faithful to the spec than the one already there.

**What I still think is worth doing**, and it is naming hygiene, not rewiring: `entryID` should
say it accepts either spelling, and the per-transfer granularity should be documented where a
caller sees it. Reversing a leg reverses that leg's **whole transfer** (`mirror` mirrors every
posting of it) - which is correct, since R1 will not let half a balanced transaction be undone,
but it is surprising from a route whose id names one leg. No route rename, no change to web.

**One small gap, advisory and low severity.** `resolveEntry` tries the transfer lookup first and
falls back to the posting lookup, and the comment justifies the ordering with "the ids are
prefixed (`xfer_`, `post_`) so the two namespaces cannot collide anyway". Nothing asserts that.
`newID(prefix)` composes `prefix + "_" + hex` and the separation is a convention held at the
call sites; **no test pins posting ids and transfer ids as disjoint.** The invariant does hold by
construction, so this is not a defect - but the stated justification for a silent ordered
fallback is unasserted, and one disjointness test would pin it.

## The conflation recurs one layer in, and no type system can see it

Raised by nia, relayed by dakari, verified here. Option 2 as a bare call-site change would
**silently** break the statement overlay:

- `reversalIndex.ts:62` keys the CORRECTION side on `row.transfer_id` - correct.
- `reversalIndex.ts:67` keys the REVERSED side on `row.id`, a **posting** id, via
  `locallyReversed.has(row.id)`, where `locallyReversed` is built from `l.reversedEntryId`.

Feed `reversedEntryId` a transfer id and line 67 compares a transfer id against a posting id.
Both are `string`, so `tsc` stays green, all 65 tests stay green, and the "Reversed" badge just
quietly stops appearing until a server round-trip. A silent failure, in the same component,
from the same conflation that caused C8.

**That is the most review-relevant fact in this burst.** The posting/transfer conflation has now
produced two defects at two layers, and in both places the type system could not help because
both ids are bare `string`. The rename (option 4) fixes this instance and leaves nia's index
carrying one id type instead of two. It does not fix the class: nothing stops the next
conflation. Branded ids - `type PostingId = string & { readonly __brand: unique symbol }`, and
the same for a transfer id, in `web/src/api/types.ts` and as distinct Go types in `core` - would
make the next one a compile error instead of a silent badge. **Worth a DEFERRED entry with this
burst as its trigger evidence**; it is not this burst's work and I am not proposing it as scope.

## C4 - CLOSED

nia deleted the alias hedge outright at `7ec09bd` (`REVERSAL_CODE_ALIASES` gone), with a test
pinning `transfer_not_found` / `entry_already_reversed` / `posting_not_found` as **UNMAPPED**
(`reversalErrors.test.ts:149-153`) so a code the server never sends cannot be silently absorbed.
Verified. Better than the advisory asked for: it closes the hedge and guards the reopening.

## C8 closure

Fixed by amara at `321398d`: `resolveEntry` accepts either spelling, trying the transfer id
first. Verified here against a real server - the posting id now returns **201**, the cross-id
double-reverse returns **409 already_reversed**, and conservation holds. C8 is dead.

## Burst verdict: APPROVE-WITH-CONDITIONS

Every blocking finding this burst raised is now closed: C1, C2, C3, C4, C5, C6, C7, C8.
The reversal path is reachable from the shipping client, correct on all six behavioural legs
plus the cross-id case, and conserves value.

**Evidence at the verdict HEAD (`d3a4a01`):** `go build` / `go vet` / `gofmt -l` clean;
web **64/64**; live reversal e2e all pass including the posting-id leg, the cross-id 409 and
conservation. Full `go test ./... -race` green (exit 0, `tests` 361s) at `321398d`; the only
code change since is `api/reversal.go`, verified **comment-only**, so that run is cited rather
than re-run (test economy).

Conditions - both now closed:

1. ~~**Naming hygiene in `core`**~~ - **CLOSED** by dakari at `7e436c4`. `api/reversal.go` now
   states at the caller-visible boundary what `{id}` accepts (a posting id, with the transfer id
   also resolving), and that reversing a leg reverses its whole transfer because R1 makes half a
   correction unrepresentable. That is the condition as written, at the place a caller reads.
2. ~~**Pin the prefix-disjointness invariant**~~ - **CLOSED** by this reviewer at Orchestrator
   instruction, in `core/id_test.go` (new file). Two tests, both **mutation-verified**:
   minting postings as `newID("xfer")` fails the disjointness test, and dropping the prefix
   from `newID` fails the namespace-tag test; both mutations applied, run and reverted. Ids are
   taken from the real minting path, never written as literals, so a prefix change cannot leave
   them stale and still passing.

   A third test - that a posting id does not resolve through `FindTransferByID` - was written
   and then **deleted**: the fake store cannot exhibit that failure, so it passed under every
   mutation. A test that cannot fail does not earn its keep, and keeping it would have
   overstated the coverage.

   **Scope note:** I hold no claim and my stream is `docs/`. `core/` is amara's claimed scope,
   and I wrote a new file in it on explicit Orchestrator instruction, after dakari and the
   other builders had stood down. Recording it because an unannounced write into another
   operator's scope is exactly what this review exists to flag - including when it is mine.

And one DEFERRED candidate, explicitly not this burst's scope: **branded ids**. The
posting/transfer conflation produced two defects at two layers in this burst, and in both the
type system could not help because every id is a bare `string`. That is the class fix; the
rest of this burst fixed instances.

---

# Review round 6 - C9, surfaced by nia after both conditions closed

**Reviewed at:** `c814880` (dev), 2026-11-23 UTC.

## C9 - OPEN. A correction stops reading as a reversal after a page reload.

Raised by nia in her closing rows; **verified here against a real `ledgerd` + real SQLite**,
not taken on report:

```
POST /entries/<posting id>/reverse   -> 201
     response body: reverses_entry_id = post_000001a209cf4247fd91f82d71a79c8f

GET  /accounts/<id>/statement        (the same data, reloaded)
     posting keys: [account_id, amount, created_at, currency, id, seq, transfer_id]
     reverses_entry_id present:    False
     reversed_by_entry_id present: False
```

The link exists **only in the reverse endpoint's own response**. `ReversesEntryID` is a field
of that response type (`api/types.go:96`); `postingResp`, which the statement is built from,
carries neither link field. So the badge and caption come entirely from the session overlay.
After a reload the overlay is gone and the correction renders as **a bare second transfer**,
with the original unmarked.

That contradicts the burst's own stated deliverable. nia's stream plan required:

> *"The compensating entry reads AS a reversal in the statement, linked to the entry it
> reverses, not as an unexplained second transfer."*

It does - for one session. Not across a reload, which is the normal case for anyone reading a
statement.

**This is the stub pattern for the third time**, and that is the part worth carrying forward.
nia's `reverse-stub.mjs` *injected* `reverses_entry_id` / `reversed_by_entry_id` into statement
responses, so the web layer was built and verified against a contract the real API never
offered. The same self-confirming shape produced C8, and I flagged it about this very stub in
round 1. Each time, only contact with the real server exposed it.

**Not fixed here.** It spans `api/` (dakari) and probably `core/` (amara), both stood down, and
the fix is a contract change - which postings carry which link fields - not a patch. Routing is
the Orchestrator's.

**Severity:** no money moves wrongly, nothing is corrupted, and every invariant still holds.
It defeats a stated deliverable of the burst rather than a correctness property, so I am
recording it as open rather than reopening the money-path verdict.

**Evidence at `c814880`:** full `go test ./... -race` **green, exit 0** (`tests` 367s) with the
two new invariant tests included; `go build`, `go vet`, `gofmt -l` clean; web 64/64; live
reversal e2e all legs pass with conservation intact.

## Burst verdict: APPROVE-WITH-CONDITIONS, one condition now open again

- C1-C8: **closed**.
- Condition 1 (naming hygiene): **closed** by dakari at `7e436c4`.
- Condition 2 (pin prefix disjointness): **closed** by this reviewer at `c814880`,
  mutation-verified.
- **C9: OPEN.** The reversal UX deliverable is met in-session only.

My view, since the Orchestrator routes it: C9 should be fixed in this burst rather than
deferred. The burst's goal was reversal, a correction that does not read as a correction after
a reload is the feature only half-delivered, and the fix is a known contract addition to a
response shape two streams already understand. But it is a deliverable gap, not a safety one,
and shipping with it recorded is a defensible call that I am not making unilaterally.

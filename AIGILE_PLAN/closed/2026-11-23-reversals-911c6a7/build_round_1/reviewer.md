<!-- AIGILE-RECORD
kind: reviewer
stage: build
round: 1
schema_version: 1.1.0
superseded_by: none
-->

# Reviewer-seat adversarial review - burst `reversals` (build stage, round 1)

SOURCE. This record carries the independent adversarial review performed by operator
`zola` across 8 rounds, published on dev at `docs/operators-review-reversals.md`
(latest round commit `d3fe0b6`). It is reproduced here because the burst manifest needs
the critique bound to it, not because the review was performed by this skill.

INDEPENDENCE, VERIFIED NOT ASSERTED. `ag_claims.check_independence` over every claim
event in this clone's manifests, against the 33-file burst diff:

    zola    PASS   holds no intersecting live claim
    amara   BLOCK  holds a live claim on core/**, internal/sqlite/**
    dakari  BLOCK  holds a live claim on api/**
    kofi    BLOCK  holds a live claim on migrations/, tests/
    nia     BLOCK  holds a live claim on web/**

zola is the ONLY eligible reviewer of this burst, and holds no claim. That is the
property the gate exists to enforce, and it holds here.

SEAT RESOLUTION FAILED - REPORTED, NOT WORKED AROUND. `ag-resolve-role --json Reviewer`
returns `state: PROFILE_STALE` (profile `codex-opencode`): "a seat this profile RESOLVES
has changed, so the frozen binding no longer describes what would run". Its named
mid-burst route is `ag-driver profile refreeze Reviewer`, which rewrites governance
routing. This stream did NOT run it: the staleness follows the canonical install moving
(16.21 -> 16.23 this session), not an intended seat edit, and re-freezing governance
routing mid-burst is an Orchestrator decision, not a builder's. So no model-family seat
was dispatched for this record; the independent review it carries is zola's.

## Verdict

APPROVE-WITH-CONDITIONS. C1-C9 all closed. Conditions 1 and 2 closed. Condition 3 open,
non-blocking, with a named fix.

## Findings closed during the burst (zola's rounds 1-8)

- C1 - reversal idempotency key minted per call, not per intent; a retry could double-post.
  CLOSED. Confirmed empirically from two sides: a fresh key on an already-reversed entry
  returns 409 `already_reversed`, because the guarantee is the derived id and does not
  depend on the key.
- C2 - ambiguous failures asserted "was not reversed" though a post-commit 500 is possible.
  CLOSED by nia.
- C3 - live reversal e2e impossible until dakari's endpoint landed. CLOSED; dakari's
  `POST /entries/{id}/reverse` is on dev and was exercised against a real server here.
- C5 - a claim commits, the compensating transfer fails, and `UNIQUE
  (original_transfer_id)` bricks that entry forever. CLOSED. kofi fixed the caller half;
  this stream's `Reverse` closes it structurally by doing the check and the postings in ONE
  `InTx`, so there is no commit-then-release window at all.
- C8 - spec authority. CLOSED.
- C9 - a correction stops reading as one after reload. CLOSED by nia's `95ba46b`,
  independently verified in the browser by this stream against the real endpoint with the
  stub DISABLED, including the reload, and FALSIFIED against the pre-fix module
  (pre-fix 1,1 -> 0,0,0; post-fix 1,1 -> 1,1,2 on the same script and seed).

## Condition 3 (OPEN, non-blocking) - the `rev_` prefix is duplicated with nothing binding it

Four literal sites, not three; two Go tests fail under mutation, not one. zola recorded
three and one, this stream raised the correction, and zola verified and published it:

    core/reversal.go:18                    const reversalPrefix = "rev_"      <- source of truth
    core/reversal_test.go:314              "rev_"+orig.ID                     <- copied literal (amara)
    internal/sqlite/reversal_test.go:370   "rev_"+orig.ID                     <- copied literal (amara), the missed one
    web/src/views/reversalIndex.ts:59      const REVERSAL_ID_PREFIX = "rev_"  <- copied literal (nia)

Mutating `reversalPrefix` to `"reversal_"` fails `TestReverse_AcceptsAPostingIDOfTheEntry`
and `TestReverse_RealAdapter_ResolvesAPostingID` (both subtests); web stays 73/73. BOTH Go
failures are for the WRONG reason - they hardcode the literal - so a developer fixes two
literals, sees green, and ships a build where core emits `reversal_` while web strips
`rev_`. Statement links stop resolving with every suite green.

Both copied Go literals are this stream's, including the one in the real-adapter test
added specifically to catch what fakes hide.

Named fix (zola): a contract test reading core's constant and asserting the TS literal
matches. Core half is narrower - the two Go literals read `reversalPrefix`;
`core/reversal_test.go` is `package core` and can read it directly, while
`internal/sqlite/reversal_test.go` is `package sqlite_test` and needs it exported or
reached through a core helper. The core half alone does NOT close the drift path.

## The transferable lesson, adopted by zola with attribution

The tell is not the duplication, it is that the duplicate sits in the TEST. A test that
restates a constant another layer owns cannot fail for the contract, only for itself - it
converts a guard into decoration while still showing green. Checked against all four of
this burst's silent cross-layer findings (C8, the overlay keying, the stub injection,
condition 3) and it holds for every one.

Its sibling, from the C9 root cause: a harness stub that INJECTS a field production never
sends makes every UI pass answer the stub's own question. The live statement emits neither
`reverses_entry_id` nor `reversed_by_entry_id`.

## zola's own recorded error

zola searched for `reversalPrefix` across Go and for `rev_` only under `web/src`, never for
the literal `"rev_"` across Go, so the fourth site was outside the reach of every search
run - and a count was then published from it. `### A search is never scoped by accident` is
a standing rule and this review is where it is meant to be enforced. Recorded as zola's
error in round 8, not as a detail of the finding.

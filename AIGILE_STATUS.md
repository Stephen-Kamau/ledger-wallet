# AIGILE_STATUS.md

The "where are we right now" index. Thin by design. Plan content lives in `AIGILE_PLAN/`; closed-burst history in `AIGILE_HISTORY.md`; lessons in the managed rules block of `CLAUDE.md` once they bind, and in `AIGILE_PLAN/BACKLOG.md` while their disposition is open.

## Current state

**Phase:** Planning
**Active burst:** None
**Burst base:** 29bfb97ef68b929a66b087a18db0572bc1919ada
**Plan reference:** None
**Next action:** kofi stream landed (0fb6617). Awaiting dakari's POST /entries/{id}/reverse; then zola re-reviews the full burst diff.
**Last updated:** [YYYY-MM-DD]

<!-- Next action is the forward pointer for resume-on-/clear. Every phase  -->
<!-- transition updates it. A new session reads this field via             -->
<!-- `ag-status --quiet` and picks up exactly where we left off. Must  -->
<!-- be accurate before any /clear - stale pointers are worse than none.   -->
<!--                                                                       -->
<!-- Plan reference points at AIGILE_PLAN/current/ when a burst is active. -->
<!-- The plan content itself is NOT copied here - AIGILE_PLAN/ is the      -->
<!-- canonical home; STATUS is just the index.                             -->
<!--                                                                       -->
<!-- current/ is a STORE: records are addressed by (kind, stage, round)    -->
<!-- from a closed vocabulary and no caller picks a filename. Resolve the   -->
<!-- newest round's plan with `ag-gate resolve-plan`, NOT by reading        -->
<!-- current/plan.md - that root-level file is a round-0 mirror nothing     -->
<!-- rewrites, and the schema declares it a non-record.                     -->
<!-- Contract: docs/standing/plan-store-schema.md                                    -->
<!--                                                                       -->
<!--                                                                       -->
<!-- Burst base is the SHA of HEAD captured at /ag-plan approval - the     -->
<!-- parent of the burst's first commit. /ag-probe reads it and passes     -->
<!-- `--base <sha>` to `codex-review adversarial-review` so the review     -->
<!-- sees only burst-scope changes.                                        -->
<!--                                                                       -->
<!-- Codex steps: NEVER write `Invoke /codex:<cmd> ...` in Next action.    -->
<!-- The next assistant will Skill-route it and hit the harness bug.       -->
<!-- Write the Bash form `Run codex-review <subcmd> ... with brief at      -->
<!-- <path>`. For rescue pointers, include --resume or --fresh explicitly  -->
<!-- when an in-session rescue thread may already exist. See AIGILE_RULES  -->
<!-- ### Codex invocation for the full rule.                               -->

**Burst id:** f565fc1d330f0ff8a3324a7dfa11f370f4ead547
**Planning started:** 2026-11-23T12:46:05Z

<!-- Concurrent operators in burst `reversals`. STATUS carries ONE active burst id,   -->
<!-- so this pointer is necessarily last-writer; the superseded ids are recorded here -->
<!-- rather than erased. Each operator's full plan record and signed manifest survive -->
<!-- intact under AIGILE_PLAN/current/bursts/<burst-id>/ and .aigile/manifests/.      -->
<!--   nia    5ec0edc10d933a51b167ce5293af1a9ef75c7de8  planning started 12:44:21Z    -->
<!--   kofi   58487fc04e63b14d4067564b4902dad8da2d0238  planning started 12:46:55Z    -->
<!--   dakari f565fc1d330f0ff8a3324a7dfa11f370f4ead547  planning started 12:46:05Z    -->
## Active streams

None.

## Ship Reviews

<!-- Populated by /ag-ship when a dev->main PR is opened. -->
<!-- Two states: WAITING_REVIEW (not yet reviewed) and WAITING_RESPONSE (reviewed, findings pending). -->
<!-- Cleared when Orchestrator records a disposition (fix/defer/skip). -->
<!-- Format:
     **PR #N:** WAITING_REVIEW | opened <ISO> | retries: 0
     **PR #N:** WAITING_RESPONSE | reviewed <ISO> | reviewed_sha: <sha> | findings: HIGH: x, MEDIUM: y
     On disposition: remove entry; record outcome in HISTORY as **Ship-review:** <outcome> -->

## Deferred Reviews

<!-- Populated by /ag-plan when you defer a code review item. -->
<!-- Cleared by /ag-probe steer checkpoint when addressed or explicitly dismissed. -->
<!-- Format: - PR #N: [comment summary] — deferred [date] -->

## Session Checkpoint

<!-- Written by hand before /compact, per CLAUDE.md Context economy. -->
<!-- Read on resume to re-orient. -->
<!--                                                                       -->
<!-- Mostly superseded by AIGILE_PLAN/current/iteration.md within an       -->
<!-- active burst. Kept for compact-time session orientation hints that    -->
<!-- don't belong in plan iteration.                                       -->

## History

<!-- Append completed Bursts. Keep last 5 entries. Full archive in AIGILE_HISTORY.md. -->
<!-- Long-term burst sequence lives in AIGILE_PLAN/long-term/sequence.md. -->

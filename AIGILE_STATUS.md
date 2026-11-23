# AIGILE_STATUS.md

The "where are we right now" index. Thin by design. Plan content lives in `AIGILE_PLAN/`; closed-burst history in `AIGILE_HISTORY.md`; lessons in the managed rules block of `CLAUDE.md` once they bind, and in `AIGILE_PLAN/BACKLOG.md` while their disposition is open.

## Current state

**Phase:** Producing
**Active burst:** reversals
**Burst base:** 911c6a70f8ca57b8cfd3bc6493b27f43a02f51d0

<!-- WHY THIS CLONE'S PAIR NAMES ONLY amara (AGV-328/329).                           -->
<!-- AIGILE_PLAN/current/bursts/ holds FOUR stores, one per operator clone, because   -->
<!-- the shared branch carries everyone's plan records while STATUS carries ONE pair:  -->
<!--   5ec0edc1...  nia     web/**                        (1 declared glob)           -->
<!--   58487fc0...  kofi    migrations/, tests/           (4 declared globs)          -->
<!--   f565fc1d...  dakari  api/**                        (1 declared glob)           -->
<!--   a1bd5f52...  amara   core/**, internal/sqlite/**   (9 declared globs)  <- THIS  -->
<!-- Each owner was read from that store's own round_1/plan.md, not inferred.         -->
<!--                                                                                  -->
<!-- RESOLUTION: this clone's pair names amara, and that is not a preference. The      -->
<!-- canonical burst-axis resolver (lib/ag_active_burst.py, THE one chokepoint per     -->
<!-- AGV-328) resolves store_dir for THIS clone to bursts/a1bd5f52..., so every        -->
<!-- consumer that addresses the burst axis here - claims, overlap, cost, render -     -->
<!-- already reads amara's store. Writing any other id would make STATUS disagree      -->
<!-- with the resolver and with .aigile/manifests/911c6a70... , the signed claim.      -->
<!-- So the closed record from THIS clone carries a1bd5f52... / base 911c6a70... .     -->
<!--                                                                                  -->
<!-- What is NOT claimed: that this is the burst id for the `reversals` burst as a     -->
<!-- whole. There is no such single id. Plan-store state is per-clone and is           -->
<!-- deliberately NOT merged across operators (AGV-328/329, and docs/REPORT-ledger-    -->
<!-- wallet-build.md "Honest limitations"). Four closed records will exist, one per    -->
<!-- clone, each naming its own id. Anyone reconciling the burst reads all four        -->
<!-- stores above, never this single field.                                           -->
<!--                                                                                  -->
<!-- Per-operator burst bases, same last-writer caveat as the burst id above.         -->
<!--   nia    5977f4196ef666a6454646ffad573d0c9b1de02d                                -->
<!--   kofi   29bfb97ef68b929a66b087a18db0572bc1919ada                                -->
<!--   dakari 26479e742078400a2872b17c33d71bcd7a1ea30f                                -->
<!--   amara  911c6a70f8ca57b8cfd3bc6493b27f43a02f51d0                                -->
<!-- amara rebased onto github/dev four times to land behind nia/zola/kofi/dakari, so -->
<!-- 911c6a7 is no longer reachable from HEAD. The signed manifest for it is          -->
<!-- committed under .aigile/manifests/ and remains the record of what was claimed.   -->
<!-- kofi hit the same thing: a claim's base SHA does not survive a rebase.           -->
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

**Burst id:** a1bd5f528ac875670d574084d0c7c8ecc5500b51
**Planning started:** 2026-11-23T12:41:13Z

<!-- Concurrent operators in burst `reversals`. STATUS carries ONE active burst id,   -->
<!-- so this pointer is necessarily last-writer; the superseded ids are recorded here -->
<!-- rather than erased. Each operator's full plan record and signed manifest survive -->
<!-- intact under AIGILE_PLAN/current/bursts/<burst-id>/ and .aigile/manifests/.      -->
<!--   nia    5ec0edc10d933a51b167ce5293af1a9ef75c7de8  planning started 12:44:21Z    -->
<!--   kofi   58487fc04e63b14d4067564b4902dad8da2d0238  planning started 12:46:55Z    -->
<!--   dakari f565fc1d330f0ff8a3324a7dfa11f370f4ead547  planning started 12:46:05Z    -->
<!--   amara  a1bd5f528ac875670d574084d0c7c8ecc5500b51  planning started 12:41:13Z    -->
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

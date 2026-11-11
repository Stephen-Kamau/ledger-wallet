# AIGILE_STATUS.md

The "where are we right now" index. Thin by design. Plan content lives in `AIGILE_PLAN/`; closed-burst history in `AIGILE_HISTORY.md`; lessons in the managed rules block of `CLAUDE.md` once they bind, and in `AIGILE_PLAN/BACKLOG.md` while their disposition is open.

## Current state

**Phase:** Idle
**Active burst:** None
**Burst base:** None
**Plan reference:** None
**Next action:** Idle - run /ag-plan to start a burst
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

## Active streams

<!-- Per-stream executor block (seven fields). Populated by /ag-plan       -->
<!-- Step 7 at approval and /ag-stream when adding parallel streams. The   -->
<!-- stream's scope and file list lives in                                 -->
<!-- AIGILE_PLAN/current/round_<N>/plan.md, not here.                      -->
<!--                                                                       -->
<!-- Single-stream bursts use stream name `main`. Multi-stream bursts use  -->
<!-- the names from the plan.                                              -->
<!--                                                                       -->
<!-- Format: one block per stream, one line per field.                    -->
<!--                                                                       -->
<!--   **Stream <name> <field>:** <value>                                  -->
<!--                                                                       -->
<!-- The admitted <field> labels come from lib/ag-stream-fields.tsv, the   -->
<!-- ONLY declared home for them, and are never restated here. Write the   -->
<!-- labels marked `current`. A field outside that table is reported as    -->
<!-- unrecognised, never parsed into a stream name.                        -->
<!--                                                                       -->
<!-- Values come from their own declared homes and are not restated here   -->
<!-- either: executor names from global/cast-roster.yaml, status values    -->
<!-- from lib/ag-stream-status.tsv. This template used to offer ASSIGNED / -->
<!-- RUNNING / HANDED-OFF / FAILED - a set no validator admitted - and it  -->
<!-- also listed the field labels, which drifted from the parser's set.    -->
<!--                                                                       -->
<!-- The status vocabulary is NOT a literal reuse of docs/standing/BUILDER_CONTRACT -->
<!-- .md's IN PROGRESS|DONE|STUCK|FAILED - it's a partial overlap only.    -->
<!--                                                                       -->
<!-- `risk-tier` and `tier-basis` were REMOVED 2026-08-17. They existed    -->
<!-- solely to feed `ag-gate risk-tier` and `ag-gate executors`, both      -->
<!-- deleted by the simplification burst; without those gates the two      -->
<!-- fields were metadata nothing read.                                    -->
<!--                                                                       -->
<!-- Renaming a label in lib/ag-stream-fields.tsv renames it in every      -->
<!-- parser at once - that is the point of the table - but it invalidates  -->
<!-- every AIGILE_STATUS.md already on disk, so treat a rename as a        -->
<!-- migration. This template used to warn specifically that `handoff      -->
<!-- head` must not be renamed because scripts/ag-burst-base parsed that   -->
<!-- exact string; it no longer does, so the warning is now the general    -->
<!-- one above. Do not fold `handoff head` into `artifact`: a SHA and a    -->
<!-- report/diff-range path are different things.                          -->
<!--                                                                       -->
<!-- Mid-burst /ag-stream additions and intra-burst /ag-handoff swaps      -->
<!-- still write the legacy triple (builders/handoff head/status) as of    -->
<!-- this schema version - full write-side migration is deferred. When     -->
<!-- both shapes are present on one stream, `executor` wins over           -->
<!-- `builders`, and the `status` line is authoritative for status.        -->

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

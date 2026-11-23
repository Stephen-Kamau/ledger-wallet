<!-- AIGILE-RECORD
kind: feedback
stage: plan
round: none
schema_version: 1.1.0
superseded_by: none
-->

# Human added value - burst `reversals`, amara stream

CAPTURE CAVEAT. `hooks/hav-capture` is NOT in this clone's `.claude/settings*.json`, so the
buffer only begins at 19:04:11Z and holds 4 lines for a burst that ran from 12:30Z. Per
`CLAUDE.md` `### HAV redirect capture` an incomplete buffer during an active burst is a
mechanism defect, not evidence that nothing happened, so the entries below the first two are
reconstructed from this session and labelled as such.

FILTER DEFECT (AGV-017 class). Buffer lines 1 and 2 are `<cross-session-message ...>`
payloads from zola, not Orchestrator prompts. A peer message is categorically not a human
turn, and the capture filter let both through. Reported, not read past.

## 2026-11-23T19:04:11Z | round_1 | correction

Stand-down lifted for ONE finding: fix the reversal link rendering in `web/**` using the
`rev_<transfer id>` derivation, verify in the browser against the real endpoint with the
stub DISABLED, and **reload the page as part of the check**. Do not touch `api/**`.

The reload clause is the whole value. Without it the check passes on the session overlay and
proves nothing: pre-fix the badges render after the click and vanish on refresh. It is what
turned a confirmation into a falsification (pre-fix 1,1 -> 0,0,0; post-fix 1,1 -> 1,1,2) and
what exposed the general defect class - the harness stub INJECTED `reverses_entry_id`, so
every earlier browser pass answered a question the stub had supplied the answer to.

## 2026-11-23T19:36:59Z | round_1 | scope

"You need to run probe and finish the whole thing." Moved the stream from stood-down to a
full probe pass; nothing was going to run it otherwise.

<!-- routine: 2026-11-23T19:18:32Z -->
<!-- routine: 2026-11-23T19:21:28Z -->

## 2026-11-23T~18:05Z | round_1 | correction (RECONSTRUCTED - outside buffer window)

"`ops git fetch origin dev`. Your clone's `origin/dev` ref is stale at 76e9e9f, so any
ahead/behind reasoning you do is wrong - your work is landed."

A direct factual correction of a real error. I had been computing ahead/behind against a ref
that had not moved since session start and reporting those numbers. Nothing in my own checks
would have caught it, because the ref was internally consistent - just stale.

## 2026-11-23T~18:20Z | round_1 | scope (RECONSTRUCTED - outside buffer window)

"Repair `AIGILE_STATUS.md` before transitioning ... decide which burst id the closed record
carries and say so in the commit message - this is the AGV-328/329 collision, so record how
you resolved it rather than quietly picking one."

Named the collision, and required the resolution be recorded rather than silently made. The
"say so" clause is what produced the evidence-based answer (the canonical burst-axis resolver
already resolves this clone to `a1bd5f52...`) instead of a preference.

## 2026-11-23T~17:30Z | round_1 | direction (RECONSTRUCTED - outside buffer window)

"keep" - in answer to whether core should write rows into kofi's `reversals` table.

Chose to keep both mechanisms and preserve kofi's boot-path drift guard, declining the wiring
that would have required `embeddedSchema` to learn `reversals` and so deleted that guard.

## 2026-11-23T~17:00Z | round_1 | scope (RECONSTRUCTED - outside buffer window)

"Dakari is waiting for you please."

Redirected me from reporting to unblocking a peer. Checking my error codes against nia's
shipped UI for dakari's benefit is what found the `transfer_not_found` vs `entry_not_found`
seam defect - before dakari built on it.

<!-- AIGILE-RECORD
kind: feedback-review
stage: probe
round: none
schema_version: 1.1.0
superseded_by: none
-->

# Feedback review - burst `reversals`, nia stream (web/**)

NO SCORES, AND WHY. The six-dimension scoring in the rubric is produced at `/ag-probe`
retrospective Step 4. **No `/ag-probe` ran for this stream.** Probe is burst-level and the
closer's; it ran once in the container repo over the whole burst (`3a524df`), and the
operator pack never assigned probe to an operator. There is therefore no reviewer pass to
record here, and inventing dimension scores to fill the shape would be fabricating the one
thing this record exists to attest. Stated rather than left blank so a later reader does
not read the absence as an oversight.

What follows is the honest residue: which captured entries were real human value, assessed
against what actually shipped.

HAV: yes

## Assessment

**Two entries changed what shipped, and both were questions.**

`H1 "everything done?"` is the highest-value turn of this stream by a distance. I had
already reported completion. The question caused me to re-read `origin/dev` and find
zola's verdict carrying two BLOCKING conditions against my own stream, which had landed
while I was writing that report. Nothing notified me, and no gate I had run was red. Had
the Orchestrator accepted my report, the burst would have shipped with a per-call
idempotency key and copy asserting a financial fact the client cannot know.

`H2 "choose the best please"` is second, and is a different shape: the Orchestrator
refused to pick between options I had framed and handed the decision back. That produced
`95ba46b`. The value was not in the choice - I would have recommended the same one - but in
being made to own it rather than park a known gap behind a surfaced-and-deferred note.

**One entry's value was in refusal.** `H7 "/ag-ship"` was an instruction I did not carry
out, and the burst is better for it. The gate evidence was unambiguous (exit 87, a burst
base absent from this clone) and both routes to satisfying it required an operator to
mutate shared release state. Recording it as HAV because the exchange, not the gate, is
what resolved it: the gate refused, and the Orchestrator's subsequent "use the best
recommendation" routed ship to the party that owns `main`.

**Three entries were peer traffic, not Orchestrator redirects** (H4, H6, and the peer half
of the thread). They are captured because the rubric captures turns. H6 is the notable one:
a peer caught a wrong claim of mine that no gate would have, and I withdrew it after
verifying each point rather than conceding on report.

## The pattern this stream should be read for

Four claims were asserted in this burst without their consequence being tested - two mine,
two amara's. Mine: that stub-injected statement fields were real, and that empty claim
globs disable review independence. The first cost a requirement-level defect that survived
two review rounds and three of my own browser passes, because the stub supplied the data
under test. The second I published to both the closer and the Orchestrator before anyone
read the function.

The generalisation, which amara is carrying into the write-up: a contaminated identifier
produces failures on a spectrum, and the loud end is the safe end - so when auditing for
identifier contamination, look hardest at the paths that keep working. `ag-phase archive`
and `ag-plan-overlap` both DEGRADED silently in this clone; `ag-gate ship-preflight`
REFUSED. Only the two that kept working could have caused harm.

And the correction to the obvious lesson: the pattern is "check it", not "docs lie". Of the
four claims, the documentation was honest in one. A rule drawn from the other three would
have been confidently false.

## Capture mechanism

A defect, and not mine to fix: the capture hook is USER-level and `turns.jsonl` is
UNTRACKED, so it archives with nothing. The buffer caught 4 of this stream's turns and
missed roughly seven hours including H1 and H2 - the two that mattered. dakari filed
`AGV-002` for the hook with `defeat: dependency`; the untracked-buffer half is the same
class. See `feedback.md` for the full caveat.

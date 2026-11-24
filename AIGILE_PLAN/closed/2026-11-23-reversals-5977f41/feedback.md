<!-- AIGILE-RECORD
kind: feedback
stage: plan
round: none
schema_version: 1.1.0
superseded_by: none
-->

# Human added value - burst `reversals`, nia stream (web/**)

CAPTURE CAVEAT, stated before the entries so nothing below is read as a full buffer.
`hooks/hav-capture` is configured at USER level (`~/.claude/settings.json`), not in this
clone, and `AIGILE_PLAN/current/turns.jsonl` is UNTRACKED - which is why it was never
archived with the store. It holds 4 turns of this session (`fa0829fb`), the earliest at
`2026-11-23T20:17:49Z`, for a stream that began at its plan commit `5977f41`
(`2026-11-23T15:49:25+03:00`). So the buffer misses roughly seven hours, including the two
redirects that mattered most. Those are reconstructed from this session's own record and
are marked RECONSTRUCTED; the four from the buffer are marked BUFFER.

Per the rubric an empty or partial buffer during an active burst is a defect in the capture
mechanism, never evidence that no redirect occurred.

## Entries

### H1 - RECONSTRUCTED - "everything done?"
Category: verification challenge. Phase: build/close.
I had reported the stream complete. The question alone caused me to re-read `origin/dev`,
where zola's review verdict had landed while I was writing that report, carrying TWO
BLOCKING conditions against my stream (C1 idempotency key minted per call, C2 copy
asserting "was not reversed" on outcomes the client cannot know).
**Materially affected what shipped: yes, decisively.** Without it I would have stood down
with both unaddressed. Fixed in `ee6b4c1`.
Lesson I take from it: "my five pack steps are done" is not "my stream is done" - a review
verdict can land after you declare completion, and nothing notifies you.

### H2 - RECONSTRUCTED - "choose the best please"
Category: delegated decision. Phase: build.
Put to the Orchestrator as fix-now versus ship-a-known-gap, after I found that the live
statement emits no reversal link fields and my stub had been injecting them. The
Orchestrator declined to choose between my options and told me to pick.
**Materially affected what shipped: yes.** I chose to fix; it produced `95ba46b`, the
`rev_` link derivation, which is on `main`. Shipping the gap would have meant every
reloaded statement rendering a correction as an unexplained second transfer - the exact
thing this stream's requirement exists to prevent.

### H3 - RECONSTRUCTED - "did you run probe and ship?"
Category: verification challenge. Phase: close.
Neither was mine; probe had been run burst-level by the closer and ship had not happened.
The question established that, and also surfaced that my last functional commit sat after
what I had misread as the review anchor - a flag I later WITHDREW as wrong (see H6).
Materially affected what shipped: no. Affected the record: yes.

### H4 - BUFFER 2026-11-23T20:17:49Z - dakari's coupling critique relayed
Category: peer challenge, not an Orchestrator redirect.
Logged because the rubric captures the turn, not because it redirected the burst. It
argued my web-only link fix hardcodes core's id convention. I agreed it is the weaker end
state, noted the precedence mitigation (explicit server fields rank above the derivation,
so a later `postingResp` field needs zero web rework), and changed nothing.

### H5 - BUFFER 2026-11-23T20:45:49Z - "To finish: archive your own plan store"
Category: scope instruction. Phase: close.
Produced the hand archive (`9e8d670`) and the manifest finalisation (`d03929b`).
**Materially affected the record: yes** - and the instruction's own wording ("as amara
did") is what made me check how she did it, which is how I found that `ag-phase archive`
would have resolved to HER store in this clone.

### H6 - BUFFER 2026-11-23T20:49:50Z - amara's correction relayed
Category: peer correction, accepted.
My review-anchor flag was wrong: I read `911c6a7` as the archive anchor when it is amara's
burst base, and the archive directory is merely NAMED after it. Withdrawn at `479053a`
after verifying each point myself. This is the one entry where the human/peer layer caught
an error of mine that no gate would have.

### H7 - BUFFER 2026-11-23T20:54:34Z - "/ag-ship"
Category: instruction REFUSED on gate evidence. Phase: ship.
I was told to ship from this operator clone. `ag-gate ship-preflight` exited 87,
`AG_GATE_RESOLUTION_UNREADABLE`, because STATUS names a burst base that does not exist
here (`git rev-parse --verify 911c6a70... -> fatal: Needed a single revision`). Both routes
to a green gate - fetch another clone's base, or rewrite a shared `AIGILE_STATUS.md` - are
an operator mutating release state to make a gate pass. I declined both and handed ship to
the closer.
**Materially affected what shipped: yes, by preventing something.** DEV FLOW step 6 would
have run close-out C1, which in this clone resolves to amara's store and would have swept
dakari's and kofi's then-open stores into a wrongly-named directory.

## What the Orchestrator contributed that no gate did

Both of H1 and H2 are questions, not corrections - "everything done?" and "choose the
best". Neither told me anything I could not have found; both made me go and look. The two
defects they surfaced (a review verdict I had not read, and a requirement failing on
reload) were invisible to every gate that ran green at the time.

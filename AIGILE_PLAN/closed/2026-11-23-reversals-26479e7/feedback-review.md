<!-- AIGILE-RECORD
kind: feedback-review
stage: probe
schema_version: 1.1.0
superseded_by: none
-->

# HAV review - burst `reversals`, dakari stream (`api/**`)

Normally written at `/ag-probe` retrospective Step 4, scoring the redirects captured
in `feedback.md` across the six dimensions in `docs/hav-classification-rubric.md`.

**No `/ag-probe` ran for this stream, and none should have.** Probe is burst-level and
belongs to the closer: it ran once over the whole burst (artefacts at `3a524df`,
steer and retrospective at `6bed6b3`), not per operator clone. This stream's
verification was its own - `go test ./api/...`, `go vet`, and two live end-to-end runs
against a real `ledgerd` and real SQLite, one of which found C8.

This file exists because `ag-phase archive` requires it (exit 80) and the operator pack
directs this stream not to run `/ag-plan`, which is what would have created the
placeholder. It is written rather than faked: no dimension scores are recorded, because
no retrospective scored them.

## What the redirects actually cost

Of the four redirects in `feedback.md`, none required rework. Two were scope rulings
that stopped this stream writing outside `api/**` (into `core/` and `web/`), and both
were correct - the items belonged to amara and nia and were closed by them.

The one redirect that changed an outcome was #1, "do so": relaying C8 directly to the
other operators rather than holding it for a routing turn. Amara's fix landed at
`321398d` and zola reproduced it independently. Holding it would have delayed the
burst's only blocking defect by at least one cycle.

## The correction worth carrying

Not a redirect, so not scored above, but it is the burst's real finding and it would
otherwise survive only in commit messages: **both catches that reshaped decisions came
from an operator re-examining a claim they had already made in public, and neither came
from a review stage.** This stream withdrew its own per-leg foreclosure argument, which
led nia to find the silent-overlay cost; nia overstated the review anchor, and checking
it rather than relaying it stopped a needless re-review of a closed burst. Full chain in
`docs/operators-activity-log.md`.

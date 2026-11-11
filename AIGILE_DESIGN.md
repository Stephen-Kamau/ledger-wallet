# AIGILE_DESIGN.md

How this system is BUILT. Peer of `AIGILE_CHARTER.md`, which holds vision, objectives and the
constraints a burst can violate.

## The boundary, because it is the whole point of having two files

A **hard spec** is a non-negotiable constraint a burst can VIOLATE - "no hardcoded family
dependencies in the skills layer", "role independence is by model family". You can check
whether a burst broke one.

**Design** is how the thing is built: architecture, feature sets, metric definitions,
configuration references. You cannot violate a feature list, only implement it or not.

Content drifts into the CHARTER because everything feels non-negotiable while you are writing
it. In `aigile-canonical` that drift reached 38,905 bytes of design sitting in `## Hard
Specs`, read in full at `/ag-plan` Step 1 by every burst in every managed repo - 60% of the
entire mandatory read. This file exists so that content has somewhere correct to go.

## When to read it

**Not at `/ag-plan` Step 1.** It is deliberately absent from `scripts/ag-brief-state`. If it
were in the default read it would be the CHARTER again under another name.

**Mandatory when a burst's scope names anything this file describes.** That trigger lives in
`/ag-plan` Step 1 and `/ag-challenge --plan` so it fires without anyone remembering it - "read
it when designing" is an intention, and an intention is not a mechanism.

## Sections

Add sections as the design acquires them. Suggested starting shape:

## Architecture

<!-- Components and how they fit together. -->

## Metrics

<!-- What is measured, how it is computed, and what a number means. -->

## Configuration reference

<!-- Roles, profiles, and the files that hold them. -->

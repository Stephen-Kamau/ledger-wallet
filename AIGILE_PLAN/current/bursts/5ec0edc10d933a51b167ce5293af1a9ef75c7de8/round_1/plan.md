<!-- AIGILE-RECORD
kind: plan
stage: plan
round: 1
schema_version: 1.1.0
superseded_by: none
-->

# Stream plan - nia (burst `reversals`)

Add entry reversal to the statement view that already shipped in `build-v1` (R21).
Consumes `POST /entries/{id}/reverse`, built in parallel by dakari.

- Scope (files WRITTEN):
  - `web/**`

Deliverables:

- A reverse action on each entry in the existing statement view.
- Typed failures rendered as actionable user-facing copy, never a raw error
  body: 409 already-reversed, 403 not-your-account.
- The compensating entry reads AS a reversal in the statement, linked to the
  entry it reverses, not as an unexplained second transfer.
- TypeScript, functional components, bun. Desktop widths only.

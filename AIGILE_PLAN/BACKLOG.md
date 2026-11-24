## Backlog

- [2026-11-11] **`AGV-001`** Currency conversion for cross-currency transfers - needs: a rate source; out
  of toy scope unless a stream surfaces demand.
  kind: work-item
  disposition: considering
  state: open
  preconditions: none
  closes-when: a stream needs a cross-currency transfer, OR R8 is relaxed in requirement.md.
    While core/ledger.go returns ErrCurrencyMismatch for mismatched legs (R8) and no rate
    source exists in the repo, this stays open and unstarted by design.
  verified-against-code: 2026-11-23
  evidence: VERIFIED 2026-11-23 by `reversals` retrospective [STILL-TRUE] - cross-currency
    is still rejected outright, not converted: core/ledger.go:152 (Transfer) and :240
    (PlaceHold) both return ErrCurrencyMismatch (R8). No rate source exists anywhere in the
    repo. The entry's claim holds and nothing in this burst touched it.
  bundle: ledger-scope

- [2026-11-23] **`AGV-002`** HAV capture is not wired in operator clones, and peer
  messages survive its filter. Two defects found at the reversals probe. (1)
  hooks/hav-capture is absent from this clone's .claude/settings, so turns.jsonl began
  at 19:04Z for a burst that started 12:30Z - most Orchestrator turns were never
  captured and had to be reconstructed from session memory and labelled as
  reconstructed. Per CLAUDE.md an incomplete buffer during an active burst is a
  mechanism defect, never evidence that no redirect occurred. (2) Two
  <cross-session-message> payloads from operator zola were written into the buffer as if
  they were human turns; a peer message is categorically not an Orchestrator prompt, and
  lib/hav_capture_engine.py excludes harness-injected text structurally but not this
  shape (AGV-017 class).
  <!--fields-->
  state: open
  closes-when: a fresh operator clone's turns.jsonl covers its burst's full Orchestrator turn span and holds no cross-session-message payloads.
  preconditions: none
  bundle: ledger-scope
  needs: an /ag-contribute report against canonical
  builder-family: claude
  builder-model: unrecorded
  defeat: dependency - both halves live in canonical, not here. The hook is wired by ag-operator join (clone provisioning) and the filter is lib/hav_capture_engine.py; CLAUDE.md `### Canonical edits` forbids editing either from a client repo, and the route is /ag-contribute. Nothing in this repo can close it.
  kind: work-item
  disposition: considering

## Bundles

Declare a bundle with `ag-backlog rebundle --declare <slug> --name <name> --purpose <purpose>`;
every backlog entry names one in a `bundle:` field. See
`docs/standing/backlog-lifecycle-schema.md`.
- `ledger-scope`
  name: Ledger scope
  purpose: Domain capabilities deliberately out of the toy's scope until a stream surfaces demand

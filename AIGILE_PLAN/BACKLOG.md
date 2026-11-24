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

- [2026-11-24] **`AGV-018`** The Reviewer seat resolves READY with a route_token but its
  dispatch FAILS: ag-resolve-role returns model gpt-6.1-sol and the local Codex CLI is
  authenticated to a ChatGPT account that refuses it with HTTP 400 model_not_found. A
  seat that resolves green and dies at dispatch is why the reversals burst recorded no
  machine-readable Reviewer verdict (reviews: {}). Resolution is for the Orchestrator:
  either re-route the seat to a model this account serves, or change the account the
  Codex CLI authenticates as.
  <!--fields-->
  state: open
  closes-when: ag-codex-dispatch completes a turn without a model_not_found error and ag-review records a verdict into the burst manifest reviews object.
  preconditions: none
  bundle: ledger-scope
  builder-family: n/a
  builder-model: unrecorded
  evidence: ag-codex-dispatch diagnostic frame 2026-11-24, burst reversals: status 400, code model_not_found. ag-review --dry-run resolves role=Reviewer reviewer=codex family=openai and exits 0, so the dry run cannot see the defect.
  defeat: dependency - re-routing a seat to a different model is a governance routing refreeze, the Orchestrator-owned act amara explicitly declined to take unilaterally in this burst (D-A1). A builder cannot resolve it.
  kind: work-item
  disposition: parked

- [2026-11-24] **`AGV-019`** The Ship-review seat cannot run: ag-resolve-role returns it
  READY on opencode-go/deepseek-v4.1-flash carrying requires_consent
  opencode-go-china-hosting, and no such consent is recorded anywhere in .aigile/.
  CLAUDE.md makes a Ship-review-seat verdict MANDATORY on every dev to main merge; the
  reversals promotion (PR 5, merged 1a48c1a) therefore shipped without one.
  <!--fields-->
  state: open
  closes-when: either the consent is recorded and ag-ship-review --pr N returns a verdict, or the Ship-review seat is re-routed to a transport needing no residency consent.
  preconditions: none
  bundle: ledger-scope
  builder-family: n/a
  builder-model: unrecorded
  evidence: ag-resolve-role --json ship-review on 2026-11-24 returns state READY, requires_consent opencode-go-china-hosting. grep over .aigile/ for that token returns nothing. opencode auth list shows OpenCode Go and Console keys stored, so the transport is authenticated and the consent is the only gap.
  defeat: dependency - the consent is a data-residency decision about sending this repository diff to China-hosted inference. That is the Orchestrator's to grant or refuse, not a builder's, and no amount of in-burst work substitutes for it.
  kind: work-item
  disposition: parked

- [2026-11-24] **`AGV-020`** main has NO server-side branch protection, so the
  two-contract override model has only one contract in force on this repository. The
  GitHub API returns 403 Upgrade to GitHub Pro or make this repository public. Nothing
  server-side requires a PR, a review, or refuses a force push or deletion on main: the
  local pre-commit and pre-push hooks are the entire guard, and they protect only clones
  that installed them.
  <!--fields-->
  state: open
  closes-when: GET repos/Stephen-Kamau/ledger-wallet/branches/main/protection returns 200 with required_pull_request_reviews set, or the Orchestrator records an explicit accepted-risk decision in AIGILE_DECISIONS.md that local hooks are the only contract.
  preconditions: none
  bundle: ledger-scope
  builder-family: n/a
  builder-model: unrecorded
  evidence: gh api repos/Stephen-Kamau/ledger-wallet/branches/main/protection on 2026-11-24 returns HTTP 403 Upgrade to GitHub Pro or make this repository public to enable this feature. CLAUDE.md Branch hygiene names server-side protection as one of five enforcement layers; on this repo that layer does not exist.
  defeat: dependency - enabling protection needs either a paid GitHub plan or making a private financial-services toy repository public. Both are account and disclosure decisions owned outside this repository, and a builder must not take either.
  kind: work-item
  disposition: parked

## Bundles

Declare a bundle with `ag-backlog rebundle --declare <slug> --name <name> --purpose <purpose>`;
every backlog entry names one in a `bundle:` field. See
`docs/standing/backlog-lifecycle-schema.md`.
- `ledger-scope`
  name: Ledger scope
  purpose: Domain capabilities deliberately out of the toy's scope until a stream surfaces demand

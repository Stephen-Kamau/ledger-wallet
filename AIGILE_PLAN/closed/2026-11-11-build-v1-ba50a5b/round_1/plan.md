<!-- AIGILE-RECORD
kind: plan
stage: plan
round: 1
schema_version: 1.1.0
superseded_by: none
-->

# Plan — burst `build-v1`

**Goal.** Implement the ledger-wallet backend (Go) per the frozen `architecture-v1` design,
resolving conditions A–D (see `AIGILE_DECISIONS.md`). Operators build concurrently in their own
clones with signed claims; the overlap/ownership gate adjudicates real file contention.

**Sequence head.** Follows `architecture-v1`. Charter Objectives: core, api, migrations, tests,
web, governance-gates-fire.

**Pre-existing red tests.** NO_BASELINE — no Go code yet; `/ag-probe` measures the delta.

## Streams and ownership

Each stream's `Scope (files WRITTEN)` list is the operator's declared claim globs.

### S1 core+adapter — Amara (architect)
- Scope (files WRITTEN):
  - `core/**`
  - `internal/sqlite/**`
  - `cmd/ledgerd/**`

### S2 api — Dakari (builder)
- Scope (files WRITTEN):
  - `api/**`

### S3 migrations+tests — Kofi (builder)
- Scope (files WRITTEN):
  - `migrations/**`
  - `tests/**`

### S4 web — Nia (designer)
- Scope (files WRITTEN):
  - `web/**`

### S5 review — Zola (runner → reviewer)
- No build scope. Independent review of the built diff; holds no claim intersecting the burst.

## Deliberate unassigned grab (the multi-user conflict)

`internal/sqlite/**` and `cmd/ledgerd/**` are Amara's per D-A1. **Dakari**, needing a running
server for his API, is expected to reach into `cmd/ledgerd/**` — work NOT assigned to him. Where
his declared/actual file set intersects Amara's live claim, the claim-overlap gate and the
ownership clock must fire. This is logged as a CONFLICT and adjudicated (ownership stays with
Amara; Dakari coordinates rather than overwrites).

## Order

Amara (core+adapter) first — the chain head freezes the compilable contract. Then Dakari (api)
and Kofi (migrations+tests) in parallel on it; Nia (web) after api; Zola reviews last.

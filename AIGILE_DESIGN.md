# AIGILE_DESIGN.md

How ledger-wallet is BUILT. Peer of `AIGILE_CHARTER.md` (vision, Hard Specs, constraints).
Authored by the operator roster in burst `architecture-v1`; the per-stream detail lives under
`docs/architecture/` and this file is the binding synthesis.

## Status

Architecture reviewed independently by Zola (`docs/architecture/99-review.md`): **APPROVE-WITH-
CONDITIONS**. The chain is coherent and the concurrency model is sound; four conditions
(below) must be resolved before `core`/`api`/`web` implementation starts. They are the agenda
for the first build burst.

## Architecture

Three layers plus storage, a strict `core → api → web` dependency chain:

```
  web (React/TS)            Nia    docs/architecture/20-web.md
     |  HTTP (typed client, Bearer auth)
  api (net/http)           Dakari  docs/architecture/10-api.md
     |  Ledger interface only (never Store/Tx)
  core (invariant engine)  Amara   docs/architecture/00-core.md
     |  Store interface (port)
  internal/sqlite (WAL)    [UNOWNED - condition A]
```

- **core** is storage-agnostic. Balances are derived from postings (R10), postings are
  append-only (R5). Double-entry invariants (R1–R5) are enforced here.
- **api** establishes *who is asking* (authentication), sets `TransferCmd.Authorizer`, and
  maps core's typed errors to HTTP status + stable `code` (never a bare 500). It authorizes
  nothing — core owns ownership (R7).
- **web** branches on the stable error `code`, not HTTP status; renders int64 minor units
  without float math.

### The frozen core contract (the chain's seam)

```go
type Ledger interface {
    CreateAccount(ctx, CreateAccountCmd) (Account, error)   // R6, R9
    GetAccount(ctx, AccountID) (Account, error)             // R9 — added by chain ruling
    Balance(ctx, AccountID) (Amount, error)                 // R10, derived, currency-less
    Transfer(ctx, TransferCmd) (TransferResult, error)      // R1–R8, R11
    Statement(ctx, AccountID, Page) (StatementPage, error)  // R12, keyset-paged
}
```
`Amount = int64` minor units (no floats). Balance body is composed `{amount: Balance(id),
currency: GetAccount(id).Currency}`. `Store`/`Tx` is the storage port; `Tx` exposes no
posting update/delete — append-only is enforced by the absence of a code path.

### Concurrency (R17/R18)

Single serialized writer: SQLite **WAL + `BEGIN IMMEDIATE`**, with the R3 sufficiency check
executed inside the write transaction against the live balance. The write lock spans
balance-read-then-insert, so no-double-spend (R18) follows from serialization and no-lost-
updates (R17) is structural (balances are derived). R4 idempotency is backstopped by
`UNIQUE(owner, idempotency_key)`. `busy_timeout` required on every connection. Per-account
mutexes and optimistic concurrency were rejected as over-engineering, DEFERRED behind an R19
bottleneck trigger.

**Empirical correction (build-v1, Zola finding C-2).** The design predicted reads would "dwarf"
writes under WAL. The measured R19 baseline (`tests/throughput_baseline.txt`) shows the opposite:
with modernc's pure-Go SQLite, reads run ~5x SLOWER than writes. Causes: a ~0.6ms per-query floor
in the pure-Go driver, the derived-balance read being an O(postings) `SUM` (R10), and a two-query
read path. The WAL rationale (reads do not block the writer) still holds; the relative-magnitude
claim does not. Not a correctness issue - recorded so the papers do not assert a false number.

### Schema (shared core ∩ migrations — the R24 overlap)

Three tables: `accounts`, `transfers` (with `UNIQUE(owner, idempotency_key)`), `postings`
(`UNIQUE(account_id, seq)`, append-only trigger). Numbered idempotent migrations (R14);
`0002` adds a `currencies` metadata table (R15) — `holds` was deferred to Amara since it
would touch the `Ledger` port. DDL in `docs/architecture/30-schema-migrations-tests.md`.

### Error contract (api → web)

One body shape `{error:{code,message,request_id}}`. Status split: 401 unauthenticated
(api) vs 403 unauthorized/privileged (core ownership) vs 409/422 business rejection
(inline on the form). Full table in `10-api.md` §3; the web `code → message/placement` map
in `20-web.md` §3.

## Open conditions before implementation (from the independent review)

- **A (blocking).** `internal/sqlite` adapter + `cmd/ledgerd` are unowned — they carry
  `BEGIN IMMEDIATE`, PRAGMAs, `seq` assignment, the UNIQUE→replay conversion, and
  system-account provisioning. Owner: Orchestrator/Amara.
- **B (blocking).** Freeze the `seq`-generation site: adapter assigns `MAX(seq)+1` per
  account inside `InsertTransfer` under the write lock; ratify Kofi's `UNIQUE(account_id,
  seq)`. Owner: Amara.
- **C (blocking).** R20 "accounts" plural is unmet — add `GET /accounts` (owner = bearer
  principal) and a `Ledger` list method. Owner: Dakari + Amara.
- **D (blocking).** Read-side authorization is undecided (web codes a 403 the backend never
  emits). Decide ownership checks on balance/statement reads and the 403-vs-404 shape.
  Owner: Dakari + Amara.
- **E/F (non-blocking).** Specify the UNIQUE→replay conversion layer; link the currency
  minor-unit scale (frontend const vs `currencies` table) — expose via `GET /currencies`.

## Per-stream sources

| Doc | Author | Covers |
|---|---|---|
| `docs/architecture/00-core.md` | Amara | domain model, invariants, `Ledger`/`Store`, concurrency, schema |
| `docs/architecture/10-api.md` | Dakari | endpoints, auth boundary, error mapping, idempotency |
| `docs/architecture/20-web.md` | Nia | three views, error UI, money precision |
| `docs/architecture/30-schema-migrations-tests.md` | Kofi | migrations, evolution, concurrency/throughput tests |
| `docs/architecture/99-review.md` | Zola | independent review, verdict, open questions |

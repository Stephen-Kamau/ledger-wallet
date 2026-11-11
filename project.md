# ledger-wallet — Project Brief

## What it is

A small **double-entry ledger / wallet service**. Accounts hold balances; value moves
between them as **transfers**. Every transfer is recorded as balanced debit/credit
postings, so the books always sum to zero and no balance is ever lost or invented.

The project is deliberately small but **financially honest**: it enforces the invariants a
real ledger must hold, rather than storing a mutable `balance` integer and hoping.

## Why it exists

This is a **toy vehicle for exercising multi-user AI-gile** — the `feature/multi-user`
machinery in `aigile-canonical`. It is the successor to the `url-shorten` toy, chosen to
stress the four things that one did not:

1. **A real auth boundary** — a transfer must be authorised against account ownership.
2. **Write-heavy concurrency** — concurrent transfers contend on the same balances and must
   serialize correctly (no lost updates, no double-spend).
3. **Schema migrations authored across operators** — the ledger schema evolves (currencies,
   holds, statements) and more than one operator touches the migration surface.
4. **A genuine cross-stream dependency chain** — not one shared file, but a contract that
   flows `core → api → web`, where each stream consumes the stage above it.

It is built by a **roster of signed operators working concurrently**, so AI-gile's
ownership, claim-overlap, and review-independence gates fire on real activity.

## Backend

**Go 1.24.** Standard library plus a thin HTTP router. Storage is SQLite (WAL) for the toy;
the `core` package is storage-agnostic so the invariants are testable without a database.

## The streams (who builds what)

| Stream | Owner | Scope | Depends on |
|---|---|---|---|
| core | **Amara** (architect) | `core/**` — accounts, postings, the double-entry invariant engine; the contract the chain flows from | — |
| api | **Dakari** (builder) | `api/**` — HTTP transfer/account endpoints, auth boundary | core contract |
| web | **Nia** (designer) | `web/**` — balances + statement UI | api shape |
| migrations+tests | **Kofi** (builder) | `migrations/**`, `tests/**` — schema evolution + the cross-stream test suite | shares the schema contract with core (overlap trigger) |
| review | **Zola** (runner) | no assigned stream — reviews peers, probes unassigned work | arbitrated by the gates |

## Scope boundary

In scope: accounts, balanced transfers, the invariant engine, an auth boundary on transfers,
schema migrations, a minimal balances/statement UI, a load/concurrency test.

Out of scope (toy): real currency rails, KYC, multi-node clustering, production auth
providers. These may appear as DEFERRED items if a stream surfaces them.

## Methodology

Built under **AI-gile**. The architecture is produced first, through the AI-gile cycle
(DECOMPOSE → PLAN → BUILD → VERIFY), before any production code. See `requirement.md` for
the testable requirements and `AIGILE_CHARTER.md` for Vision / Hard Specs once initialised.

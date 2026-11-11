# AIGILE_DECISIONS.md

Record of challenged/resolved design decisions. Prevents relitigating settled calls.

## build-v1 — resolving the architecture review's conditions (coordinator rulings)

These resolve the four blocking conditions from the `architecture-v1` independent review
(`docs/architecture/99-review.md`). Additive, in-scope, no CHARTER Hard Spec touched — made by
the burst coordinator and recorded here.

- **D-A1 [2026-11-11] (condition A) — `internal/sqlite` and `cmd/ledgerd` ownership.**
  Assigned to **Amara** (she owns the `Store` port/contract, so the adapter that implements it
  and the server entrypoint that wires it belong with core). Ruling: these packages are Amara's
  stream scope in `build-v1`.
  *Note: this is deliberately the overlap surface for the multi-user exercise — Dakari needs a
  running server for his API and will reach for `cmd/ledgerd`; that grab is expected to trip the
  overlap/ownership gate against Amara's claim.*

- **D-B1 [2026-11-11] (condition B) — `seq` generation + UNIQUE index.** `postings.seq` is
  assigned by the SQLite adapter inside `InsertTransfer`, under the `BEGIN IMMEDIATE` write lock
  (`MAX(seq)+1` per `account_id`). Kofi's `UNIQUE(account_id, seq)` strengthening is ratified.
  Confirms Zola's adjudication.

- **D-C1 [2026-11-11] (condition C) — R20 "accounts" plural.** Add
  `AccountsByOwner(ctx, PrincipalID) ([]Account, error)` to the `Ledger` interface and a
  `GET /accounts` endpoint (owner = authenticated bearer principal). Purely additive to the
  frozen contract; Amara owns the interface edit, Dakari the endpoint.

- **D-D1 [2026-11-11] (condition D) — read-side authorization.** Balance and statement reads
  require the authenticated principal to **own** the account. A read of an account the principal
  does not own returns **404** (not 403) to avoid leaking account existence. System/mint
  accounts are readable only by the configured admin set. The web layer drops its "403 not your
  account" state accordingly (reads are 404 or 200).

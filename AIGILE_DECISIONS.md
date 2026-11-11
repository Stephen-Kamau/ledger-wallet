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

## build-v1 — independent review findings (Zola, `docs/operators-review-build-v1.md`)

Verdict APPROVE-WITH-CONDITIONS. Dispositions (coordinator):

- **C-1 (medium) — FIXED in burst.** Adapter computed `postings.seq` but never wrote it back into
  the returned transfer result (real adapter reported `seq:0`; fakes hid it). A defect found in a
  burst is fixed in the burst — Amara fixed it with a real-adapter test the fakes cannot satisfy.
- **C-2 (medium) — RECONCILED (docs).** R19 baseline contradicted the "reads dwarf writes" design
  claim. Not a code bug; `AIGILE_DESIGN.md` concurrency section now records the empirical reality.
- **C-3 (low) — DEFERRED.** UNIQUE→replay backstop not implemented in `InsertTransfer` (returns a
  raw error on constraint violation). Near-unreachable under `BEGIN IMMEDIATE` (the in-tx
  `FindTransferByKey` fast-path handles replay). **Trigger:** if the serialized-writer model is
  ever relaxed (per-account locks / optimistic concurrency), implement the backstop conversion and
  a test that reaches it.
- **Conflict resolution — VERIFIED sound.** Zola confirmed via `go list` that no `api→cmd` or
  `core→cmd` edge exists; dependency direction `cmd→{api,core}` intact after Amara's revert.

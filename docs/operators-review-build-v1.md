# build-v1 — Independent Build Review

**Reviewer:** Zola Juma (`zola-1`), Reviewer seat.
**Independence (R24):** I built none of `core`, `internal/sqlite`, `cmd/ledgerd`, `api`,
`migrations`, `tests` or `web`. My identity differs from every author under review (Amara,
Dakari, Kofi, Nia). This is the review-independence gate firing on real signed activity.
**Scope:** the integration tree on `dev` (all streams merged), against the frozen design
(`docs/architecture/00..30`), the `architecture-v1` review conditions (`99-review.md`), the
coordinator rulings `D-A1..D-D1` (`AIGILE_DECISIONS.md`), and `requirement.md` R1-R24.
**Build/test evidence I ran myself (changed nothing):** `go build ./...` clean; `go vet
./...` clean; `go test ./...` green (api, core, internal/sqlite, tests ~11s);
`go test -race ./tests/ -run TestR17|TestR18|TestR4_Concurrent` green.

**Verdict: APPROVE-WITH-CONDITIONS.** The chain is built as designed, the four review
conditions are genuinely resolved, the concurrency model is correct and race-clean, and
requirement coverage is real (implemented AND tested), not claimed. Three conditions below
are non-blocking for a toy but are real defects/gaps a reviewer must not wave through — two
of them are places where a fake masks a divergence from the real adapter, which is exactly
the class of thing this seat exists to catch.

---

## 1. Does the built code honor the frozen design + the A-D decisions?

Spot-checks, each verified against the code, not the prose:

- **Balance is derived, never stored (R10): CONFIRMED.** No `balance` column exists
  (`migrations/0001_init.sql`; `internal/sqlite/sqlite.go` embeddedSchema). `accounts` has no
  balance field. Every balance is `SELECT COALESCE(SUM(amount),0) FROM postings WHERE
  account_id=?` (`internal/sqlite/store.go:119`, `balance()` at `store.go:191`). `core.Account`
  (`core/account.go:32`) has no balance field. There is no `UPDATE ... SET balance`.
- **Postings append-only (R5): CONFIRMED, belt-and-braces.** The `core.Tx` port exposes no
  update/delete (`core/store.go:26`); the engine only ever INSERTs. Plus storage triggers
  `postings_no_update`/`postings_no_delete` (`0001_init.sql:54-64`). Tested at both the adapter
  level (`internal/sqlite/sqlite_test.go:183`) and the migrated-schema level
  (`tests/migrations_test.go:93`), including that a compensating INSERT still works.
- **R3 sufficiency check is inside the BEGIN IMMEDIATE write tx (R18): CONFIRMED.** The check
  runs inside `store.InTx`'s closure, reading `tx.Balance` under the held write lock
  (`core/ledger.go:137-145`); `InTx` opens the tx via `BeginTx` with `_txlock=immediate` in the
  DSN (`internal/sqlite/store.go:20-33`, `sqlite.go:90`), so the write lock is taken at
  transaction start, not first write. The pre-tx reads are safe only because account fields are
  immutable (there is no account-mutation path) — correct, and the arch review's caveat holds.
- **`seq` assigned in-adapter under the lock (D-B1): CONFIRMED (for persistence).** `core`
  leaves `Posting.Seq` zero in `materialise` (`core/ledger.go:163-176`); the adapter computes
  `COALESCE(MAX(seq),0)+1 WHERE account_id=?` inside `InsertTransfer` under the write lock
  (`internal/sqlite/tx.go:87,107`). `UNIQUE(account_id,seq)` is the loud backstop
  (`0001_init.sql:49`). See Finding 1 for the one thing it does NOT do (write the assigned seq
  back into the returned Transfer).
- **api touches only the `Ledger` port: CONFIRMED.** `go list` shows `api` imports `core` and
  nothing else in-module; `Handler` holds only `core.Ledger` (`api/handler.go:12`). No handler
  references `Store`/`Tx`. `GetAccount` landed on `Ledger` (`core/ledger.go:17`), so the
  balance-body currency read stays on the inbound port.
- **Non-owned reads return 404 (D-D1): CONFIRMED.** `getBalance` and `getStatement` load the
  account, then `isAllowedReader(principal, owner)` (owner or configured admin); a non-owned
  read returns 404 `account_not_found`, never 403 — no existence leak
  (`api/handlers.go:91,158`; `api/handler.go:103`). Tested (`api/api_test.go:206,363`), plus the
  admin-can-read case (`:219`). The web layer dropped its old "403 not your account" read state
  (`web/.../transferErrors.ts:28`).
- **GET /accounts present (D-C1): CONFIRMED.** `AccountsByOwner` on `Ledger`
  (`core/ledger.go:20`) → `Store.AccountsByOwner` (`store.go:42`) → `GET /accounts`
  (`api/handler.go:84`, `handlers.go:64`), owner = authenticated principal; tested for
  non-leak (`api_test.go:164`).

All four D-decisions are honored in code, not just documented.

## 2. Requirement coverage R1-R24 (implemented AND tested)

| Req | Impl | Test | Note |
|---|---|---|---|
| R1 balanced | `buildEntries` (`transfer.go:57`) | `TestR1` (core) | zero/neg magnitude + self-transfer rejected |
| R2 conservation | emergent from R1; mint = transfer from `AllowNegative` sys acct | `TestR2`; oracle `SUM(postings)=0` (`tests/harness_test.go:171`) | — |
| R3 no negative | in-tx check (`ledger.go:137`) | `TestR3`, `TestR18`, `TestSQLite_InsufficientFunds` | real contention |
| R4 idempotent | `(owner,key)` fast path (`tx.go:52`) | `TestR4` (core/sqlite), `TestR4_ConcurrentReplaySameKey` | backstop conversion gap — Finding 3 |
| R5 append-only | no Tx mutator + triggers | adapter + migrated-schema tests | — |
| R6 ownership | set at create (`ledger.go:49`) | `TestR6` | — |
| R7 authorised debit | `from.Owner==Authorizer` pre-tx (`ledger.go:118`) | `TestR7` (core), `TestTransfer_NotOwner_403` (api) | 403 before any write |
| R8 single currency | pre-tx (`ledger.go:115`) | `TestR8` (both mismatch paths) | — |
| R9 create account | `POST /accounts` | `TestCreateAccount_*` | owner from auth, not body |
| R10 get balance | derived SUM | `TestGetBalance_*`, `TestBalance_Derived` | — |
| R11 create transfer | `POST /transfers` | `TestTransfer_Happy` + error cases | — |
| R12 statement | keyset on seq (`store.go:71`) | `TestStatement_*`, `TestSQLite_SeqAndStatement` | paging exercised |
| R13 typed errors | one `errors.Is` map, fail-closed (`api/errors.go:40`) | `TestErrorMap_EveryCoreSentinelMapped`, `_StatusAndCode`, `_UnmappedFallsToTyped500` | every sentinel covered |
| R14 numbered migrations | strictly-increasing applier (`migrate.go`) | `TestR14_ReRunIsNoOp` | idempotent, no-op re-run |
| R15 schema evolution | `0002_currencies.sql` | `TestR15_SchemaEvolved` | additive |
| R16 bootstrap parity | all-at-once vs step-by-step | `TestR16_BootstrapParity` | schema+seed+versions |
| R17 no lost updates | derived balance + serialized appends | `TestR17_NoLostUpdates` (N=200) | genuine contention |
| R18 no double-spend | BEGIN IMMEDIATE + in-tx R3 | `TestR18_NoDoubleSpend` (K=50,M=200) | exact affordable subset |
| R19 throughput | recorded baseline | `tests/throughput_baseline.txt` | numbers recorded, but analysis is wrong — Finding 2 |
| R20 balances view | `GET /accounts` + web `AccountsPane` | `TestListAccounts_OnlyOwn` + web | the arch-review hole is closed |
| R21 statement view | web `StatementPane` | web vitest | depends on R20 list — now present |
| R22 transfer form | typed error placement | `transferErrors.test.ts` coverage test | keys off `code`, not status |
| R23 signed operators | distinct signed identities | activity log / manifests | — |
| R24 gates fire | amara∩dakari overlap + this review | overlap adjudication logged | real conflict, see §4 |

**No coverage holes.** R20 — the one true hole the architecture review flagged — is now
closed end to end (port, endpoint, UI, test). Every requirement is implemented and has at least
one test at the level the property lives (core-unit for invariants, real-SQLite for
adapter/concurrency, httptest for transport).

## 3. Correctness risks

- **Concurrency claim (R17/R18): holds, and I pushed on it.** `BEGIN IMMEDIATE` takes the
  write lock at tx start, so no second writer interleaves between the R3 balance read and the
  posting INSERT. `TestR18` drives M=200 withdrawals against K=50 funds and asserts exactly 50
  succeed, 150 `ErrInsufficientFunds`, account ends at 0, never negative — and it passes under
  `-race`. `busy_timeout` (5000 adapter / 30000 tests) makes a blocked writer wait rather than
  spuriously error; infra busy is classified and retried, never folded into the invariant count
  (`harness_test.go:122,133`). I found **no race window**. The design's one real latent gap —
  the pre-tx reads being safe only because accounts are immutable — is honored (no mutation
  path exists); worth preserving as an explicit invariant if an "edit account" feature ever
  lands.
- **Idempotency replay: correct on the live path.** Under `BEGIN IMMEDIATE` the second replayer
  waits, then `FindTransferByKey` sees the committed transfer and replays via the fast path
  (`ledger.go:125-135`). `sameParams` (`transfer.go:93`) correctly rejects a same-key/different-
  params reuse as `ErrIdempotencyConflict`. Scoping is `(owner,key)`, so one client's key space
  is its own. See Finding 3 for the backstop.
- **Auth boundary: no unauthorized debit slips through.** api sets `Authorizer` strictly from
  the resolved bearer principal and never pre-checks ownership (`handlers.go:129`,
  `middleware.go:92`); core owns `from.Owner==Authorizer` and rejects pre-tx before any write.
  A missing/invalid token is 401 in middleware and never reaches a handler. `/healthz` is the
  one enumerated auth exemption (an allowlist map, not a `!=` skip — `middleware.go:33,94`),
  honoring the fail-closed rule. The `allow_negative` privilege gate is enforced at the API for
  non-admins (403 `forbidden_privileged_flag`, `handlers.go:44`). I could not construct an
  unauthorized debit that posts.
- **Money handling: integer minor units throughout, no float.** `Amount int64` end to end;
  wire is `int64` (`api/types.go`); schema `INTEGER` with `CHECK(amount>0)` on transfers. No
  float touches a balance. Two low-severity notes, both toy-safe: (a) no explicit int64
  overflow guard on `bal+debitLeg` or on the balance SUM — at extreme magnitudes SQLite `SUM`
  can promote to float and lose exactness, irrelevant at toy amounts but worth a one-line cap if
  this ever grows; (b) `created_at` is written `RFC3339Nano` by the adapter but echoed
  `RFC3339` (seconds) by the API — cosmetic, no correctness impact.

## 4. The conflict + its resolution

The overlap was real and is recorded honestly: Dakari's signed claim grabbed `cmd/ledgerd/**`
(`operators-activity-log.md:26`), his delegated build edited `cmd/ledgerd/main.go` (Amara's
file) to mount the router, and the overlap gate flagged **amara ∩ dakari on `cmd/ledgerd/**`**
(`:30`). Amara, as owner under `D-A1`, ruled the entrypoint stays hers, rejected the
`ListenAndServe`-in-her-file shape, and refolded the mount through `api.NewHandler` (`:31-35`).

**The resolution is sound.** I verified the dependency direction with `go list`, not by
eyeballing:
- `core` imports nothing in-module (stdlib only). `api` imports only `core`.
  `internal/sqlite` imports only `core`. **Nothing imports `cmd`.** `core` has no `sqlite`
  dependency.
- So the edges are `cmd → api → core` and `cmd → internal/sqlite → core`, with
  `core ← api ← cmd` preserved. There is **no `api → cmd` and no `core → cmd` edge.**
- `cmd/ledgerd/main.go` now owns only lifecycle (flags, `New` wiring via `wire.go`,
  `ListenAndServe`), and mounts `api.NewHandler(ledger, ...).Router()` (`main.go:37-40`). `api`
  exposes `NewHandler`/`Router`/`WithAdmins`/`WithTokens` and builds no server itself.

**Nothing of substance was lost in the revert.** The admin-set wiring survives (`-admins` flag
→ `WithAdmins`, `main.go:46-60`), `/healthz`, the full middleware chain and all routes are
present and tested. One minor, toy-acceptable gap: `main` has no graceful shutdown —
`defer db.Close()` is unreachable because `ListenAndServe` only returns on error into
`log.Fatalf` (which `os.Exit`s). Not a build-v1 blocker.

## 5. Test adequacy — is it real or theater?

Mostly real, with two fake-vs-real divergences the suite does not catch (Findings 1 and 3):

- **Concurrency tests exercise genuine contention.** `barrier()` releases all goroutines at
  once onto the single write lock; distinct keys in R17 mean every goroutine is a real write;
  R18 asserts the *exact* affordable subset, not just "no negative". These are not smoke tests —
  they would fail loudly under a `BEGIN DEFERRED` regression. Confirmed `-race` clean.
- **Error mapping is proven complete, fail-closed.** `TestErrorMap_EveryCoreSentinelMapped`
  makes a new unmapped `core.Err*` a failing test, and `_StatusAndCode` drives each sentinel
  through a real handler. The web mirror (`transferErrors.test.ts` + `TRANSFER_REACHABLE_CODES`)
  does the same on the UI side.
- **Migrations tests prove idempotency AND parity** (`TestR14/R15/R16`), comparing canonical
  `sqlite_schema` DDL, the currency seed, and the version set — not just "it ran".
- **Theater caveat (Finding 1):** `api_test.go` and the core unit tests drive the engine over
  in-memory fakes (`memStore`, `fakeStore`) that assign `seq` back into the returned postings
  (`fakes_test.go:120`, `fake_store_test.go:122`). The **real** adapter does not
  (`tx.go:87`). No test asserts the seq *value* on a first-write transfer result through the
  real adapter, so the divergence is invisible to the suite. `TestSQLite_SeqAndStatement` checks
  seq only via a re-read `Statement`, which does read from the DB and is correct — it does not
  touch the write-response path.
- **Theater caveat (Finding 3):** `TestR4_ConcurrentReplaySameKey` asserts the outcome (1
  transfer, 2 postings) but, as the arch review predicted, cannot distinguish fast path from
  backstop; under `BEGIN IMMEDIATE` it only ever exercises the fast path, so the UNIQUE→replay
  conversion code is effectively untested — and in fact that conversion is not implemented
  (see Finding 3).

## 6. Verdict — APPROVE-WITH-CONDITIONS

The build faithfully implements the frozen design, resolves all four `D-A1..D-D1` conditions
in code, keeps the dependency chain intact through a real ownership conflict, and passes a
race-clean concurrency suite with honest requirement coverage. It is approvable. The conditions
below are not design-invalidating and are toy-acceptable to defer, but each is a genuine defect
or gap and is stated so a later reviewer does not have to rediscover it.

**Conditions (ranked):**

- **C-1 (medium) — `seq` is not written back into the first-write transfer result.**
  `internal/sqlite/tx.go:87-98` computes the per-account `seq` and inserts it, but never assigns
  `tr.Postings[i].Seq`. So a `201` `POST /transfers` response reports `seq: 0` for every
  posting, contradicting the documented contract (`10-api.md §1.3` shows real seq values). The
  in-memory fakes DO write it back, which is why the api suite is green and the divergence is
  hidden. Fix: assign the computed seq back onto the posting in `InsertTransfer`, and add a
  real-adapter test asserting the seq on the transfer *result* (not only via `Statement`).
- **C-2 (medium) — the R19 baseline contradicts the design's central performance claim, and
  nobody reconciled it.** `00-core.md §5` and `30-schema §5.1` assert reads dwarf writes (the
  whole WAL justification). The recorded `throughput_baseline.txt` shows the **opposite**: reads
  ~266-583 ops/s vs writes ~1352-3166 ops/s — reads ~5x *slower* at every concurrency. R19's
  letter is met (a write number, a read number, zero failed invariants), but the headline
  analysis is false as recorded. Either the read measurement is flawed (e.g. `Store.Balance`
  does two queries — an existence `GetAccount` then the SUM, `store.go:63-67`) or the design
  narrative is unsupported. Reconcile the measurement or restate the claim; do not leave a
  baseline that falsifies its own design doc.
- **C-3 (low, defense-in-depth) — the UNIQUE→replay backstop conversion is unimplemented.**
  On a `UNIQUE(owner,idempotency_key)` violation, `InsertTransfer` returns a raw
  `"insert transfer: %w"` error (`tx.go:83`), not a replay and not `ErrIdempotencyConflict`.
  `BEGIN IMMEDIATE` makes this path near-unreachable, so it is not a live bug — but it is the
  explicitly-deferred `§5-E` item from the architecture review, still open, and its test cannot
  reach it. Either implement the catch-and-replay (adapter maps `SQLITE_CONSTRAINT_UNIQUE` →
  rollback → re-read → replay) with a seam that forces the backstop in a test, or record it as
  accepted, untested defense-in-depth. State the choice; do not leave it silently open.

**Minor notes (no action required for the toy):** no graceful shutdown in `cmd/ledgerd/main.go`
(`defer db.Close()` unreachable under `log.Fatalf`); no int64-overflow guard on balance
arithmetic / `SUM` (toy-safe); `created_at` written as `RFC3339Nano`, echoed as `RFC3339`.

*Zola Juma, Reviewer seat (`zola-1`). Review-independence gate (R24) satisfied: I authored none
of the streams under review. Read-only — I changed no source file; I wrote only this document.*

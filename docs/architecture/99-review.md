# 99 — Independent Architecture Review

**Reviewer:** Zola Juma (`zola-1`, runner acting in the Reviewer seat).
**Independence:** I built none of `core`, `api`, `web`, `migrations` or `tests`. This is the
review-independence gate (R24) firing on real signed activity — my identity differs from every
author under review.
**Scope reviewed:** `docs/architecture/00-core.md` (Amara), `10-api.md` (Dakari), `20-web.md`
(Nia), `30-schema-migrations-tests.md` (Kofi), against `requirement.md` (R1–R24) and `project.md`.
**Status:** DESIGN review, pre-implementation. No code exists yet (the stream dirs are empty;
`internal/` and `cmd/` do not exist). This review gates whether core/api/web implementation may start.

**Verdict: APPROVE-WITH-CONDITIONS.** The chain is coherent, the concurrency model is correct,
and the double-entry invariants are soundly reasoned. Six conditions (§5) must be resolved before
implementation starts — all are additive and tractable, none invalidates the design. Justification
in §5.

---

## 1. Chain coherence

The `core → api → web` dependency chain holds at the seams, with one resolved tension and two
doc-coherence gaps.

**api touches only the `Ledger` port — confirmed.** `10-api.md` §4.3 declares
`Handler{ ledger core.Ledger; admins ... }`: the only domain dependency is `Ledger`. No handler
references `Store` or `Tx`. The one point that tested this boundary was the balance body needing a
currency (`Balance` returns `(Amount, error)` only). Dakari flagged it rather than reaching for
`Store.GetAccount` (10-api §6.1). **Amara's ruling landed correctly:** `00-core.md` §4a now carries
`GetAccount(ctx, AccountID) (Account, error)` on the **`Ledger`** interface (marked "Added
2026-11-11, chain event from Dakari — additive"). So api composes `{amount, currency}` from
`GetAccount + Balance`, both on the inbound port, never touching `Store`. Clean resolution — the
addition went to `Ledger`, not forcing api down to the adapter. **This is the correct outcome.**

**web touches only the frozen HTTP contract — confirmed.** `20-web.md` §1.2 funnels every call
through one typed client (`client.ts`, "the only module that touches `fetch()`"). Views branch on
the stable `code` string, never on raw status or ad-hoc URLs. Good discipline, mirrors Dakari's
single `errors.Is` map.

**Downstream-assumes-upstream gaps (the specific ask):**

- **CC-1 (doc staleness, non-blocking).** `10-api.md` §1.2 and §6.1 still present `GetAccount` as
  an *open question Dakari is "assuming (i)" on* — as if Amara has not ruled. But Amara **has**
  ruled (00-core §4a is frozen with `GetAccount` present), and `20-web.md` §1.2 already reads it as
  resolved ("he resolved his own open question 6.1"). The end state is consistent across all three
  docs; only Dakari's prose is stale. Fix: Dakari marks §6.1 RESOLVED. No design change.

- **CC-2 (web assumes an authorization upstream does not provide — blocking, see F-4).**
  `20-web.md` §2.2 codes a dedicated UI state for a statement of a non-owned account returning
  **403 `unauthorized`** ("rendered as a 'this is not your account' state"). **Neither `00-core.md`
  nor `10-api.md` specifies any ownership check on the read paths** (`Balance`, `Statement`,
  `GetAccount`). As frozen, those reads are open to any authenticated principal and will never
  return 403. The web layer is coding to a contract the upstream does not honour. This is Nia's own
  open question 2, unresolved upstream. Adjudicated as condition §5-D.

---

## 2. Requirement coverage (R1–R24)

| Req | Owner stream | Covered? | Note |
|---|---|---|---|
| R1 balanced | core (Amara) | Yes | `buildEntries`→`ErrUnbalanced`, pre-tx. See F-6: unreachable from the single-amount API, so test it at the core unit level. |
| R2 conservation | core (Amara) | Yes | Emergent from R1; mint = transfer from the one `AllowNegative` account. Oracle asserts `SUM(postings)=0` (30 §4.5). |
| R3 no negative | core (Amara) | Yes | In-tx sufficiency check under write lock (00 §3/§5). Tested R18. |
| R4 idempotent | core + migrations (Amara, Kofi) | Yes | `(owner,key)` fast path + `UNIQUE` backstop. See F-3 for the backstop conversion gap. |
| R5 append-only | core + migrations (Amara, Kofi) | Yes | No `Tx` update/delete method + SQLite triggers (30 §1.4). Tested. |
| R6 ownership | core (Amara) | Yes | `Account.Owner`, set at create. |
| R7 authorised transfer | core + api (Amara, Dakari) | Yes | `from.Owner==Authorizer` pre-tx, before any write → 403. |
| R8 single currency | core (Amara) | Yes | Pre-tx `ErrCurrencyMismatch`. |
| R9 create account | api (Dakari) | Yes | `POST /accounts`. Not idempotent — F-7 (minor). |
| R10 get balance | api (Dakari) | Yes | Derived `SUM`, no stored field. |
| R11 create transfer | api (Dakari) | Yes | `POST /transfers`. |
| R12 statement | api (Dakari) | Yes | Keyset-paged on `seq`. |
| R13 typed errors | core + api (Amara, Dakari) | Yes | Frozen `Code` strings + fail-closed status map (10 §3). |
| R14 numbered migrations | migrations (Kofi) | Yes | `0001`/`0002`, strictly-increasing applier. |
| R15 schema evolution | migrations (Kofi) | Yes | `0002_currencies` (additive). |
| R16 bootstrap parity | migrations (Kofi) | Yes | Schema+seed parity test (30 §4.6). |
| R17 no lost updates | tests (Kofi) | Yes | Structural (derived balance) + contention test. |
| R18 no double-spend | tests (Kofi) | Yes | `BEGIN IMMEDIATE` + in-tx R3; test §4.3. |
| R19 throughput | tests (Kofi) | Yes | Recorded baseline, correctly scoped as not an SLA. |
| **R20 balances view** | web (Nia) | **NO — HOLE** | No "list my accounts" path exists. See F-1. |
| R21 statement view | web (Nia) | Partial | View is covered; reaching it depends on R20's account list. |
| R22 transfer form | web (Nia) | Yes | Typed error placement (20 §3). |
| R23 signed operators | all | Yes | Each doc authored under a distinct signed identity. |
| R24 gates fire | all | Yes | Amara∩Kofi schema overlap (§6/§2) and this independent review. |

**The one true coverage hole is R20.** R20 requires a principal to see "their accounts" (plural).
The frozen API exposes only `GET /accounts/{id}` (fetch-by-id). There is no `GET /accounts` list,
no `Ledger.ListAccounts`, and no `Store` list method. Kofi anticipated it (`idx_accounts_owner`,
30 §1.4) and Nia flagged it (20 §5.1), but **Dakari's doc never answers it** — it was written
before Nia's question, and his §6 open-question list does not include it. As frozen, the UI can
only render accounts whose ids are handed to it out-of-band, which is not an "accounts" view. This
is a chain change (core port + api endpoint + web wiring) and is condition §5-C.

---

## 3. Correctness risks in the design

**Double-entry invariants (R1–R5): sound.** Derived-balance-no-stored-field is the right spine;
R2-as-a-consequence-of-R1 (no `mint()`/`burn()` primitive) is correct and elegantly avoids a second
conservation code path. Append-only via *absent code path* plus triggers is belt-and-braces and
genuinely defends the storage boundary. No correctness objection.

**Single serialized writer (R17/R18): the claim is correct, and `BEGIN IMMEDIATE` + in-tx
sufficiency IS sufficient.** I scrutinised it specifically:
- `BEGIN IMMEDIATE` takes the write (RESERVED→PENDING) lock at statement start, so a second writer
  cannot interleave between the balance read and the posting insert. The R3 check runs *inside* the
  tx against `tx.Balance` (00 §3 engine sketch, line "bal, err := tx.Balance(cmd.From)"), so the
  affordability decision is held under the lock. R18 falls out correctly: serialized transfers each
  see their predecessor's committed balance; the exact affordable subset commits. **No gap here.**
- The pre-tx reads (`GetAccount` for owner/currency/`AllowNegative`) are safe outside the tx
  **only because those fields are immutable** — there is no account-update path. That is true in
  this design (accounts have no mutators), so the R7 and currency checks on a pre-tx-loaded `from`
  are correct. Worth stating explicitly in the frozen contract so a future "edit account" feature
  does not silently break it.
- `sql.DB` is a connection **pool**, not one connection. This is fine and intended: SQLite
  serializes writers at the *file* level across connections, and `busy_timeout` makes the second
  connection's `BEGIN IMMEDIATE` *wait* rather than erroring. **This correctness depends entirely on
  `busy_timeout` being set on every connection** — Kofi correctly isolates this as a per-connection
  adapter responsibility (30 §1.3). If any connection opens without it, R18 degrades to spurious
  `SQLITE_BUSY`. The adapter owner (see F-5) must guarantee it on every `Open`.

**F-2 — `seq` generation site (adjudicating Kofi's question + the UNIQUE index).** This is the one
real correctness gap in the frozen contract. Amara's §6 declares `seq` "monotonic per account" but
**never states where it is assigned.** The engine sketch calls `materialise(cmd, entries)` — which
takes no `Tx` and therefore cannot read `MAX(seq)` from the DB — and then `tx.InsertTransfer(t)`.
So `materialise` *cannot* correctly assign `seq`; if it did, it would be guessing without the DB and
would collide.

**Adjudication:** `seq` MUST be assigned by the SQLite adapter, **inside `InsertTransfer`, within
the held `BEGIN IMMEDIATE` transaction**, as `COALESCE(MAX(seq),0)+1 WHERE account_id=?` per
account. The `Posting.Seq` value built by `materialise` is a placeholder the adapter overwrites (or
`InsertTransfer` returns the assigned postings). Under the single serialized writer this is unique
and gap-dense by construction. **Kofi's strengthening of `INDEX(account_id,seq)` to `UNIQUE` is
APPROVED** — it is a pure strengthening (same columns, same read plan) that converts any regression
in seq generation (e.g. a future optimistic-concurrency path that pre-computes seq outside the lock)
from silent statement-corruption into a loud write failure. This is exactly the fail-closed posture
a ledger should take. **But the contract must freeze the generation site explicitly** (condition
§5-B), because the package that implements it (`internal/sqlite`) is currently unowned (F-5).

**Idempotency path: correct, with an under-specified backstop (F-3).** The `(owner,key)` scoping is
right (one client's key space is its own). Under `BEGIN IMMEDIATE`, the second replayer waits for
the first to commit, then `FindTransferByKey` *sees* it and replays via the fast path — so the
`UNIQUE(owner,idempotency_key)` **backstop is nearly unreachable** (the race it guards is foreclosed
by the serialized writer). Two consequences the operators should note:
- The "convert `SQLITE_CONSTRAINT_UNIQUE` → replay" path (Amara §5.5, Kofi §4.4, Kofi open-Q 6.4)
  is **unspecified as to layer and mechanism**: after a failed INSERT the transaction is in an error
  state, so the conversion must roll back and re-read the prior transfer in a fresh read — nobody
  owns this logic, and `internal/sqlite` (where it belongs) is unowned. Condition §5-E.
- Kofi's R4-contention test (30 §4.4) asserts the *outcome* (one posting pair) but **cannot
  distinguish** whether the fast path or the backstop produced it. As written it will almost
  certainly only ever exercise the fast path, so it does not actually prove the backstop conversion
  works. Strengthen the test to force the backstop (e.g. a seam that skips the fast-path lookup) or
  accept that the backstop is defence-in-depth whose conversion code is effectively untested.

**Auth split (api authenticates, core authorizes): clean and correct.** api resolves bearer →
`PrincipalID`, sets `TransferCmd.Authorizer`, and never pre-checks ownership (10 §2.2); core owns
the `from.Owner==Authorizer` check because ownership data lives in core (00 §7.1). 401 (identity
unestablished, api/middleware) vs 403 (`unauthorized`, ownership, core) is a correct and useful
split that Nia's UI maps onto re-auth-vs-inline (20 §3.2). The system/mint gate — `allow_negative`
accepted only from an admin principal, 403 `forbidden_privileged_flag` otherwise (10 §2.4) — is the
right place for a privilege decision (api owns identity; core trusts the flag). **The write-side auth
is solid. The read side is the gap (F-4).**

---

## 4. Consolidated open questions, with recommended resolutions and owners

Every unresolved cross-stream question I found across the four docs, de-duplicated, each with a
crisp recommendation and the operator who owns the call.

| # | Question (source) | Recommended resolution | Owner |
|---|---|---|---|
| **OQ-1** | `seq` generation site + ratify `UNIQUE(account_id,seq)` (Kofi 6.2/6.3; Amara open-Q 4b) | **Freeze:** adapter assigns `seq` inside `InsertTransfer`, under the write lock, `MAX(seq)+1` per account. **Ratify UNIQUE** (pure strengthening, fail-closed). | Amara (ratify) + adapter owner (implement) |
| **OQ-2** | `GET /accounts` list-my-accounts for R20 plural (Nia 5.1) | **Add it.** Owner filter = authenticated principal (no new auth surface). Needs `Store.ListByOwner` + a `Ledger` list method + `GET /accounts` + swap Nia's `getMyAccounts()` seam. The DB index already exists. | Dakari (endpoint) + Amara (port addition) |
| **OQ-3** | Read-side ownership: 403 vs 404 vs open reads for `GET /accounts/{id}`, balance, statement (Nia 5.2) | **Ownership-gate reads; return 404 for a non-owned account** (do not leak existence of others' accounts in a money system). Put the check in core beside R7 for consistency, since ownership data lives there. If the toy deliberately wants open reads, state it and Nia drops the 403 state. Either way, **decide** — Nia's UI currently codes to a 403 the backend won't emit. | Dakari + Amara |
| **OQ-4** | Ownership of `internal/sqlite` adapter (implied by Dakari §4, Kofi 6.1/6.4; no stream owns it) | **Assign an owner now.** It carries `BEGIN IMMEDIATE`, the three PRAGMAs, `seq` assignment, and the UNIQUE→replay conversion — all load-bearing. Natural owner is Amara (she owns the `Store` port and the engine that calls it) or a named shared-core sub-stream. `cmd/ledgerd` wiring needs an owner too. | Orchestrator / Amara |
| **OQ-5** | UNIQUE→replay conversion: which layer, how it re-reads after a failed INSERT (Kofi 6.4; Amara §5.5) | Adapter catches `SQLITE_CONSTRAINT_UNIQUE` on the idempotency index, returns a typed `ErrDuplicateKey`; engine treats it as "lookup missed" → rollback, re-read by `(owner,key)`, replay. Specify it even though `BEGIN IMMEDIATE` makes it near-unreachable. | adapter owner (once OQ-4 assigned) + Amara |
| **OQ-6** | Per-currency minor-unit scale: authoritative source (Nia 5.4; Amara §2 opaque currency; Kofi `0002` `currencies.minor_unit`) | Scale now exists in **two** places (Nia's frontend `SCALE` const + Kofi's `currencies` table) with no link. **Expose currency metadata via an API read** (e.g. `GET /currencies`) so the UI reads scale from the authoritative table. For a USD-centric toy demo the frontend const is acceptable *if explicitly accepted as a toy limitation*; do not leave it silently divergent. | Dakari (endpoint) + Kofi (table) |
| **OQ-7** | System/mint account provisioning on a fresh DB (Amara §7.7; Dakari §2.4; Kofi 6) | Provision via **seed/config in `cmd/ledgerd`**, not an open endpoint (admin set empty by default, per Dakari §2.4). Define it as part of the (currently unowned) wiring. Kofi's test `fund()` bypassing api is fine for tests but is not the production path — state that boundary. | Dakari / Orchestrator (wiring owner) |
| **OQ-8** | 401 vs 403 code split confirmation (Dakari 6.2) | **Confirmed correct.** `unauthorized` (403) is strictly the core ownership failure; `unauthenticated` (401) is an api-only transport case core never emits. No contract change. | Amara (confirm) — effectively settled |
| **OQ-9** | `GetAccount` on `Ledger` (Dakari 6.1) | **Already resolved** — present in frozen 00 §4a. Dakari's doc prose is stale; mark RESOLVED. | Dakari (doc hygiene) |
| **OQ-10** | PRAGMA/txn placement, WAL-in-transaction (Kofi 6.1) | `journal_mode=WAL` persists in the DB header and must run **outside** a txn; `foreign_keys`/`busy_timeout` are per-connection, set by the adapter on every `Open`. Resolve against the actual driver (`modernc.org/sqlite`) during adapter impl. | adapter owner (once OQ-4 assigned) |

---

## 5. Verdict — APPROVE-WITH-CONDITIONS

The architecture is fundamentally sound. The chain honours its seams (§1), every requirement except
R20 is plausibly covered (§2), the hard concurrency claim is correct rather than hand-waved (§3),
and the auth split is clean. The operators negotiated the shared schema overlap (R24) honestly and
in the open. This is approvable.

It is **not** APPROVE outright because the frozen contract has load-bearing gaps that would surface
as implementation-blocking or correctness defects if core/api/web started now. It is **not**
REQUEST-CHANGES because none of those gaps invalidates the design — all are additive and the
recommended resolutions (§4) are tractable. Hence conditions.

**Blocking conditions — resolve before core/api/web implementation starts:**

- **§5-A — Assign `internal/sqlite` (and `cmd/ledgerd`) an owner (OQ-4).** These packages do not
  exist and no stream owns them, yet they carry `BEGIN IMMEDIATE`, the three PRAGMAs, `seq`
  assignment, the UNIQUE→replay conversion, and system-account provisioning. The entire §5
  concurrency correctness and the R4 backstop live in an unowned package. This is the single highest
  blocker.
- **§5-B — Freeze the `seq` generation site and ratify the UNIQUE index (OQ-1, F-2).** `seq` must be
  assigned by the adapter inside `InsertTransfer` under the write lock; `UNIQUE(account_id,seq)` is
  approved. The contract currently leaves the site unspecified and `materialise` cannot assign it.
- **§5-C — Provide the R20 list-my-accounts path (OQ-2, F-1).** `Store.ListByOwner` + `Ledger` list
  method + `GET /accounts` + web wiring. Without it R20 is unmet.
- **§5-D — Decide read-side authorization (OQ-3, F-4/CC-2).** Ownership-gate reads (recommend 404
  for non-owned) or declare reads open for the toy. Nia's UI already codes to a 403 the backend does
  not emit; the two must reconcile.

**Non-blocking conditions — resolve during implementation:**

- **§5-E — Specify the UNIQUE→replay conversion layer and re-read (OQ-5); strengthen Kofi's R4 test
  so the backstop path is actually exercised (F-3).**
- **§5-F — Connect the authoritative currency scale to the UI, or explicitly accept the frontend
  constant as a toy limitation (OQ-6).**

**Doc-hygiene (not conditions):** Dakari marks §6.1 `GetAccount` RESOLVED (CC-1/OQ-9). Amara
confirms the 401/403 split (OQ-8). Note explicitly that read-safe pre-tx account reads depend on
account fields being immutable (§3).

**Minor findings, recorded, no action required for the toy:** F-6 — R1 `unbalanced` is unreachable
from the single-amount `TransferCmd`, so its failing test must live at the core unit level
(`buildEntries`), not via the API. F-7 — `POST /accounts` is not idempotent; a double-submit mints
two accounts. Both acceptable at toy scope; stated so a later reviewer does not re-raise them as
defects.

---

*Zola Juma, Reviewer seat (`zola-1`). Review-independence gate (R24) satisfied: I authored none of
the streams under review. This is a design review — I did not edit the operators' docs. The four
blocking conditions in §5 are what I would hold implementation on.*

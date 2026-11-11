# 00 — Core Architecture (the chain's keystone)

**Author:** Amara Okeyo (`amara-1`, architect) — owner of the `core/` stream, HEAD of the
`core → api → web` dependency chain.
**Status:** FROZEN contract. Dakari (`api`), Nia (`web`) and Kofi (`migrations`/`tests`)
build against the interfaces in §4 and the schema in §6. Changes to §4 or §6 are a chain
event — raise them with me, do not fork them.
**Module:** `github.com/Stephen-Kamau/ledger-wallet` · **Go:** 1.24 · stdlib + thin router only.

This is a design document. It freezes types, interfaces, and the enforcement strategy. It
does not implement the packages — the Go below is the normative *sketch* downstream codes to,
not the final source.

---

## 1. System overview

A double-entry ledger that is honest by construction: balances are **derived** from an
append-only posting log, never stored and mutated. Three layers, each consuming the one above
it, plus a storage adapter behind a port so `core` never imports a database.

```
                        ┌──────────────────────────────────────────────┐
   HTTP client ───────▶ │  api/        HTTP + AUTH BOUNDARY (Dakari)    │
                        │  - authenticates the caller → PrincipalID     │
                        │  - decodes/encodes JSON, maps typed errors    │
                        │    (R13) to status codes                      │
                        └───────────────────┬──────────────────────────┘
                                            │ depends on core.Ledger (§4)
                                            ▼
                        ┌──────────────────────────────────────────────┐
   web/ (Nia) ────────▶ │  core/       INVARIANT ENGINE (Amara)        │
   consumes api shape   │  - domain types: Account/Posting/Transfer     │
                        │  - enforces R1–R8, returns typed errors       │
                        │  - STORAGE-AGNOSTIC: talks only to Store (§4)  │
                        └───────────────────┬──────────────────────────┘
                                            │ depends on core.Store port
                                            ▼
                        ┌──────────────────────────────────────────────┐
                        │  internal/sqlite/   Store adapter             │
                        │  - SQLite + WAL, BEGIN IMMEDIATE writes (§5)  │
                        │  - schema from migrations/ (Kofi, §6)         │
                        └──────────────────────────────────────────────┘
```

The arrows are compile-time dependencies. **`core` depends on nothing but the stdlib.** It
defines the `Store` port; the SQLite adapter depends on `core`, not the reverse. This is what
makes the invariants testable without a database: a fake `Store` in a unit test exercises
every rule in §3.

### Go package layout

```
ledger/
├── core/                 # Amara — the frozen contract lives here
│   ├── account.go        #   Account, AccountID, PrincipalID, Currency
│   ├── posting.go        #   Posting, Entry, Amount
│   ├── transfer.go       #   Transfer, TransferCmd, TransferResult
│   ├── ledger.go         #   Ledger interface (§4) + the concrete engine (§3)
│   ├── store.go          #   Store + Tx ports (§4)
│   └── errors.go         #   LedgerError + typed sentinels (R13)
├── api/                  # Dakari — HTTP handlers, auth boundary, error mapping
├── internal/
│   └── sqlite/           # SQLite Store implementation (adapter)
├── migrations/           # Kofi — numbered idempotent SQL (schema of §6)
├── web/                  # Nia — balances / statement / transfer UI
├── tests/                # Kofi — cross-stream + concurrency (R17–R19)
└── cmd/ledgerd/          # main(): wires sqlite.Store into core.New(), mounts api
```

`core` is the only package the rest of the chain imports for domain meaning. `internal/sqlite`
is unreachable outside this module by design — nobody codes to the adapter, everybody codes to
the port.

---

## 2. The core domain model

Money is **integer minor units** (`int64`). No floats touch a balance, ever — floating point
is not associative and a ledger that is not associative is not conserved. Currency is carried
alongside the amount and is a dumb scale label for the toy (no conversion, R8).

```go
package core

// Amount is a signed quantity in the smallest unit of its currency (e.g. cents).
// A debit is negative, a credit is positive. Amounts only have meaning paired with
// a Currency; core never mixes currencies within a transfer (R8).
type Amount int64

type Currency string      // ISO-ish label, opaque to core: "USD", "GBP". Not validated beyond non-empty.
type AccountID string     // opaque id minted by core (ULID-like); never the owner.
type PrincipalID string   // the owning principal; the auth subject (R6).

// Account is an owned balance holder. It stores NO balance field — the balance is
// derived from postings (R10). allowNegative is the single, explicit escape hatch for
// the system/mint account (R3); every other account is floored at zero.
type Account struct {
	ID            AccountID
	Owner         PrincipalID
	Currency      Currency
	AllowNegative bool      // true only for designated system/mint accounts
	CreatedAt     time.Time
}

// Entry is an INTENDED leg of a transfer — the request-level unit. A transfer is a set
// of entries that must net to zero (R1). Entries are validated, then persisted as Postings.
type Entry struct {
	Account AccountID
	Amount  Amount        // signed: negative debits, positive credits
}

// Posting is the PERSISTED, immutable record of an applied leg (R5). Once written it is
// never updated or deleted — a correction is a new compensating transfer. Seq gives a
// total order within an account for statements (R12).
type Posting struct {
	ID         string
	TransferID string
	Account    AccountID
	Amount     Amount
	Currency   Currency
	Seq        int64         // monotonic per-account ordering for statements
	CreatedAt  time.Time
}

// Transfer is the unit of intent and the idempotency unit (R4). It records the
// materialised movement; its Postings are the proof it balanced.
type Transfer struct {
	ID             string
	IdempotencyKey string        // client-supplied, unique per owner (R4)
	Owner          PrincipalID   // the authorising principal (R7)
	From, To       AccountID
	Amount         Amount        // the positive magnitude moved (From→To)
	Currency       Currency
	CreatedAt      time.Time
	Postings       []Posting     // exactly the balanced legs that were written
}
```

**Balance is a function, not a field.**

```go
// Balance(account) == Σ posting.Amount for that account. There is no other source of truth.
```

The separation of `Entry` (intent) from `Posting` (fact) is deliberate: it is what lets the
engine validate and reject *before* anything is written (R1, R7 — "before any posting is
attempted").

---

## 3. The double-entry invariant engine

All enforcement lives in `core`, in one place: `ledgerEngine.Transfer`. The engine is the only
writer of postings. Each rule has a defined check site and a typed failure (R13). The ordering
matters — cheap, caller-visible rejections (auth, currency, balance-of-legs) happen **before**
the transaction opens; fund sufficiency happens **inside** it, under the write lock (§5),
because it depends on live balance.

| Rule | What it guarantees | Check site | On violation returns |
|---|---|---|---|
| **R1** balanced | the transfer's entries sum to **exactly zero** | pre-tx, in `buildEntries` | `ErrUnbalanced` |
| **R8** single currency | all legs + both accounts share one currency | pre-tx | `ErrCurrencyMismatch` |
| **R7** authorised debit | `from.Owner == cmd.Authorizer` | pre-tx, after account load, **before any write** | `ErrUnauthorized` (403-class) |
| **R4** idempotent | same key ⇒ original result, no second post | inside tx, first thing | replay: original `TransferResult`; conflicting params: `ErrIdempotencyConflict` |
| **R3** no negative | debited normal account stays ≥ 0 | inside tx, after reading live balance | `ErrInsufficientFunds` |
| **R5** append-only | postings never mutated | structural — engine only ever INSERTs; `Store`/`Tx` expose no update/delete | n/a (no code path exists) |
| **R2** conservation | ledger-wide sum is invariant | **emergent from R1** — every transfer nets zero, so the grand total cannot move except across the `AllowNegative` system account (mint/burn *is* a transfer from it) | n/a (structural) |

Two of these deserve emphasis because they are where toy ledgers cheat:

- **R2 is not a runtime check; it is a consequence of R1.** There is no `mint()` or `burn()`
  operation. Minting is a plain `Transfer` whose `From` is the system account (the only account
  with `AllowNegative = true`). Because that transfer still nets to zero, the invariant "Σ all
  balances is constant" holds by construction; the system account simply goes negative by the
  amount in circulation. A conservation *test* (Kofi, R19) asserts Σ balances == 0 after any
  run — that is a test of R1's integrity, not a separate rule.

- **R3 must be evaluated inside the same transaction that writes the postings**, reading the
  balance under the write lock. Checking the balance in a separate read, then writing, is the
  classic double-spend race (R18). See §5.

Engine sketch (illustrative, not final):

```go
func (e *ledgerEngine) Transfer(ctx context.Context, cmd TransferCmd) (TransferResult, error) {
	// --- pre-transaction: cheap, pure, no writes -------------------------------
	entries, err := buildEntries(cmd)        // R1: must net to zero, else ErrUnbalanced
	if err != nil { return TransferResult{}, err }

	from, err := e.store.GetAccount(ctx, cmd.From)
	if err != nil { return TransferResult{}, err }   // ErrAccountNotFound
	to, err := e.store.GetAccount(ctx, cmd.To)
	if err != nil { return TransferResult{}, err }

	if from.Currency != to.Currency || from.Currency != cmd.Currency {
		return TransferResult{}, ErrCurrencyMismatch        // R8
	}
	if from.Owner != cmd.Authorizer {
		return TransferResult{}, ErrUnauthorized            // R7 — before any write
	}

	// --- transaction: serialized writer, R3/R4 under the write lock (§5) --------
	var result TransferResult
	err = e.store.InTx(ctx, func(tx Tx) error {
		if prior, ok, err := tx.FindTransferByKey(cmd.Authorizer, cmd.IdempotencyKey); err != nil {
			return err
		} else if ok {
			if !sameParams(prior, cmd) { return ErrIdempotencyConflict }
			result = resultOf(prior)        // R4: replay, no second post
			return nil
		}
		if !from.AllowNegative {
			bal, err := tx.Balance(cmd.From) // live balance, held under write lock
			if err != nil { return err }
			if bal+debitLeg(entries) < 0 { return ErrInsufficientFunds } // R3
		}
		t := materialise(cmd, entries)       // builds Transfer + its Postings
		if err := tx.InsertTransfer(t); err != nil { return err } // append-only INSERT (R5)
		result = resultOf(t)
		return nil
	})
	return result, err
}
```

`CreateAccount` is a thin, single-INSERT path (no invariant beyond "owner and currency are
set", R6). `Balance` and `Statement` are read-only and may run outside a write transaction.

---

## 4. The frozen core contract — the SEAM the chain consumes

**This section is the interface Dakari codes to and Kofi's adapter satisfies. It is frozen.**
Two ports: `Ledger` (what `api` calls inward) and `Store` (what `core` calls outward, so
SQLite is swappable and the engine is unit-testable with a fake).

### 4a. `Ledger` — inbound port (api depends on THIS)

```go
// Ledger is the entire domain surface api is allowed to touch. api does auth and transport;
// everything financial happens behind this interface. All errors are typed (§ errors, R13).
type Ledger interface {
	// CreateAccount mints an account for an owner in a currency (R6, R9).
	CreateAccount(ctx context.Context, cmd CreateAccountCmd) (Account, error)

	// GetAccount returns the account record (R9). It carries Currency and Owner, so api
	// composes a well-formed balance body {amount, currency} from GetAccount + Balance,
	// and renders Nia's balances/statement headers, without ever touching the Store/Tx
	// port. Returns ErrAccountNotFound for an unknown id. (Added 2026-11-11, chain event
	// from Dakari — additive, no behaviour change to existing methods.)
	GetAccount(ctx context.Context, id AccountID) (Account, error)

	// Balance returns the derived balance (Σ postings) for an account (R10). The amount
	// is currency-less by design — pair it with GetAccount's Currency for a body.
	Balance(ctx context.Context, id AccountID) (Amount, error)

	// Transfer performs a balanced, authorised, idempotent transfer (R1–R8, R11).
	// Replaying an idempotency key returns the original result with no second posting (R4).
	Transfer(ctx context.Context, cmd TransferCmd) (TransferResult, error)

	// Statement returns an account's postings in Seq order, paged (R12).
	Statement(ctx context.Context, id AccountID, page Page) (StatementPage, error)
}

type CreateAccountCmd struct {
	Owner         PrincipalID
	Currency      Currency
	AllowNegative bool          // system/mint provisioning only; api must gate who may set this
}

type TransferCmd struct {
	Authorizer     PrincipalID   // the AUTHENTICATED caller (api sets this; core checks ownership, R7)
	From, To       AccountID
	Amount         Amount        // positive magnitude moved From→To
	Currency       Currency
	IdempotencyKey string        // client-supplied (R4); required
}

type TransferResult struct {
	Transfer Transfer
	Replayed bool                // true when returned from an idempotency replay (R4)
}

type Page struct { Limit int; AfterSeq int64 }               // keyset pagination on Posting.Seq
type StatementPage struct { Postings []Posting; NextAfterSeq int64; HasMore bool }

// New constructs the engine. This is the only wiring point; main() passes the SQLite Store.
func New(store Store) Ledger
```

### 4b. `Store` — outbound port (core depends on THIS; sqlite implements it)

```go
// Store is the storage-agnostic port. core NEVER imports a DB driver; it talks only to Store.
// InTx is the serialization primitive (§5): the SQLite adapter runs fn inside a single
// BEGIN IMMEDIATE write transaction, so every read fn makes is held under the write lock and
// the whole closure commits or rolls back atomically.
type Store interface {
	InTx(ctx context.Context, fn func(tx Tx) error) error

	// Read-only paths (may run outside a write tx).
	GetAccount(ctx context.Context, id AccountID) (Account, error)
	Balance(ctx context.Context, id AccountID) (Amount, error)
	Statement(ctx context.Context, id AccountID, page Page) (StatementPage, error)
}

// Tx is the transactional surface. It exposes INSERTs and reads but DELIBERATELY NO update or
// delete of postings — append-only (R5) is enforced by the absence of a code path, not by
// convention.
type Tx interface {
	GetAccount(id AccountID) (Account, error)
	Balance(id AccountID) (Amount, error)                       // live, under the write lock
	InsertAccount(a Account) error
	FindTransferByKey(owner PrincipalID, key string) (Transfer, bool, error) // R4 lookup
	InsertTransfer(t Transfer) error                            // writes transfer + its postings atomically
}
```

Design notes downstream must honour:

- **`InsertTransfer` writes the transfer row and all its postings in one call** so the adapter
  can keep them in a single SQL transaction frame. The engine never inserts a posting without
  its transfer.
- **The idempotency lookup is keyed by `(owner, key)`**, not key alone — one client's key space
  is its own. The schema enforces this with a UNIQUE constraint (§6); the engine's
  `FindTransferByKey` is the fast path, and the UNIQUE constraint is the backstop that makes the
  race in §5 safe even if two goroutines pass the lookup.
- **Context flows through.** Cancellation and deadlines reach the DB; adapters must respect it.

---

## 5. Concurrency model (R17 no lost updates, R18 no double-spend)

This is the Hard concern, so I am being concrete and I am making the call.

**Chosen strategy: a single serialized writer, realised as SQLite WAL + `BEGIN IMMEDIATE`, with
the sufficiency check (R3) performed inside the write transaction.**

Why this and not the alternatives:

1. **SQLite already permits exactly one writer at a time.** Fighting that with application-level
   sharded locks buys nothing for the toy and adds a second source of truth about ordering. I
   will not pretend to parallelise writes that the storage engine serializes anyway.
2. **`BEGIN IMMEDIATE` acquires the write lock at transaction start, not at first write.** This
   is the whole game. The naive `BEGIN` (DEFERRED) takes a read lock first and only upgrades on
   the first write — two transactions can both read the balance, both decide the debit is
   affordable, and one upgrade fails with `SQLITE_BUSY` *after* the decision. `BEGIN IMMEDIATE`
   forecloses that: the second transfer against the account blocks until the first commits, then
   reads the *already-updated* balance. Read-balance-then-write is therefore atomic with respect
   to every other writer.
3. **R3 is checked inside that transaction, against the live balance.** Because the balance read
   happens under the held write lock, the "affordable" decision cannot be invalidated by a
   concurrent transfer. This is what makes R18 fall out: N transfers that each individually pass
   R3 but together overdraw are serialized; each sees the balance left by its predecessor; the
   exact affordable subset commits and the rest return `ErrInsufficientFunds`.
4. **R17 (no lost updates) is structural here** because balances are *derived*, not written.
   There is no `UPDATE accounts SET balance = ...` to lose — concurrency correctness reduces to
   "every posting INSERT lands and is counted", which serialized appends guarantee.
5. **R4 under contention:** two goroutines replaying the same key race on `FindTransferByKey`.
   Serialized writes mean the first commits the transfer; the second, now holding the write lock,
   sees it and replays. The `UNIQUE(owner, idempotency_key)` constraint is the backstop: if the
   lookup ever misses, the second INSERT fails the constraint and the engine converts that into a
   replay rather than a duplicate post.

**Tradeoffs, stated honestly:**

- **Cost:** write throughput is capped at one transfer at a time. For the toy and R19's baseline
  this is correct-first and more than adequate; the predecessor url-shorten toy showed reads
  dwarf writes, and reads here do not take the write lock.
- **Busy handling:** set a `busy_timeout` so a blocked writer waits rather than erroring; the
  adapter owns this. A transfer that still times out is a retryable infrastructure error, **not**
  an invariant failure — it must surface as such so R19 records "zero failed invariants".
- **Rejected — per-account mutexes in Go:** would allow concurrent transfers on *different*
  accounts, but a transfer touches two accounts, so you need ordered multi-lock acquisition to
  avoid deadlock, and you still need the DB transaction underneath. Complexity for throughput the
  toy does not need. **Deferred** with trigger: if R19 shows the single writer is the bottleneck
  and the toy is asked to scale, revisit with either per-account locks or an optimistic
  `version` column + retry.
- **Rejected — optimistic concurrency (version column, compare-and-set, retry loop):** scales
  better but adds retry logic and a stored version that is a second truth about balance. Against
  the "derived balance, no mutable field" Hard Spec, it is the wrong default. Same deferral
  trigger as above.

Reads (`Balance`, `Statement`) run outside the write transaction. WAL lets them proceed
concurrently with the single writer without blocking it — this is why WAL specifically, not
rollback-journal mode.

---

## 6. The schema contract — **SHARED WITH THE MIGRATIONS STREAM (Kofi)**

> **This is the deliberate overlap point (R24).** I define the logical schema `core` requires;
> **Kofi owns `migrations/` and turns this into numbered, idempotent migration files (R14).**
> Treat the table/column names and constraints below as the contract between us. If Kofi needs
> to change a name or type, it is a chain event — reconcile with me, because `internal/sqlite`
> reads these exact columns. Types are SQLite affinities.

### `accounts`
| column | type | constraint | notes |
|---|---|---|---|
| `id` | TEXT | PRIMARY KEY | opaque AccountID |
| `owner` | TEXT | NOT NULL | PrincipalID (R6) |
| `currency` | TEXT | NOT NULL | opaque label (R8) |
| `allow_negative` | INTEGER | NOT NULL DEFAULT 0 | 1 only for system/mint (R3) |
| `created_at` | TEXT | NOT NULL | RFC3339 UTC |

### `transfers`  (also carries the idempotency key — see decision below)
| column | type | constraint | notes |
|---|---|---|---|
| `id` | TEXT | PRIMARY KEY | TransferID |
| `owner` | TEXT | NOT NULL | authorising principal (R7) |
| `idempotency_key` | TEXT | NOT NULL | client-supplied (R4) |
| `from_account` | TEXT | NOT NULL REFERENCES accounts(id) | |
| `to_account` | TEXT | NOT NULL REFERENCES accounts(id) | |
| `amount` | INTEGER | NOT NULL | positive magnitude, minor units |
| `currency` | TEXT | NOT NULL | (R8) |
| `created_at` | TEXT | NOT NULL | RFC3339 UTC |
| | | **UNIQUE(`owner`, `idempotency_key`)** | the R4 backstop (§5) |

### `postings`  (append-only — R5)
| column | type | constraint | notes |
|---|---|---|---|
| `id` | TEXT | PRIMARY KEY | PostingID |
| `transfer_id` | TEXT | NOT NULL REFERENCES transfers(id) | |
| `account_id` | TEXT | NOT NULL REFERENCES accounts(id) | |
| `amount` | INTEGER | NOT NULL | signed; debit<0, credit>0 |
| `currency` | TEXT | NOT NULL | |
| `seq` | INTEGER | NOT NULL | monotonic per account, for statements (R12) |
| `created_at` | TEXT | NOT NULL | RFC3339 UTC |
| | | **INDEX(`account_id`, `seq`)** | drives `Balance` SUM and `Statement` keyset paging |

**Decision I made as HEAD — idempotency keys live on `transfers`, not a separate table.**
The deliverable named four conceptual tables (accounts, postings, transfers, idempotency keys).
I fold the idempotency key into `transfers` with `UNIQUE(owner, idempotency_key)` rather than a
standalone `idempotency_keys` table because the key has a 1:1 relationship with a transfer and
no lifecycle of its own — a separate table would be a second row to keep consistent with the
transfer for zero benefit. **Kofi:** if you have a reason to split it (e.g. to record in-flight
keys before the transfer commits), raise it; otherwise freeze it folded.

**Append-only enforcement (R5) — recommended, Kofi's call on mechanism.** `core` never emits an
`UPDATE`/`DELETE` on `postings` (the `Tx` port has no such method). For defence in depth I
recommend a migration-level SQLite trigger that raises on `UPDATE OF`/`DELETE ON postings`. That
makes R5 enforced at the storage boundary too, not only by `core`'s discipline. Flagging it as a
schema-stream decision since it lives in a migration.

**Balance query (what the index serves):**
```sql
SELECT COALESCE(SUM(amount), 0) FROM postings WHERE account_id = ?;   -- R10, derived, no cache
```
No materialised balance column. A balance *cache* table is **deferred** (trigger: R19 shows the
SUM is the read bottleneck); if ever added it is a cache rebuilt from postings, never the source
of truth — adding a mutable authoritative balance would violate the Hard Spec.

### Schema evolution (R15) — a note for Kofi, not a constraint I own
R15 wants at least one post-v1 evolution. The natural one that `core` would consume is a
`holds`/`reserved` concept (authorise now, capture later), which would add a `holds` table and a
"available balance = Σ postings − Σ active holds" rule. **I am not freezing that now** — it is a
v2 decision. If Kofi (or the Orchestrator) wants the evolution to be `holds`, I will extend the
`Ledger` contract with a `Hold`/`Capture` pair in a follow-up; until then the v1 contract above
stands and the evolution can be the simpler `currency` metadata migration R15 also permits.

---

## 7. Open questions / decisions for the chain

Ordered by who they block.

1. **[Dakari — auth split] Authentication is api's; authorization is core's.** api authenticates
   the caller and sets `TransferCmd.Authorizer` to the proven `PrincipalID`. `core` enforces
   `from.Owner == Authorizer` and returns `ErrUnauthorized` (R7), which api maps to **403**.
   core does **not** trust api to have checked ownership — ownership data lives in core, so the
   check lives in core. api must never set `Authorizer` to anything but the authenticated
   subject. Confirm you are good with this boundary.

2. **[Dakari — error mapping, R13] Typed errors are the contract, status codes are yours.**
   `core` returns typed `LedgerError` values with stable `Code` strings. Proposed mapping:
   `unbalanced`→422, `currency_mismatch`→422, `insufficient_funds`→409, `unauthorized`→403,
   `account_not_found`→404, `idempotency_conflict`→409. Never a bare 500 for a known rule.
   The `Code` strings are frozen with the interface; the HTTP status mapping is yours to own.

3. **[Dakari — idempotency replay is a success, not an error] `TransferResult.Replayed == true`
   returns the original transfer with a 2xx (I suggest 200 on replay vs 201 on first write).
   Do not surface replay as a client error.

4. **[Kofi — schema overlap, R24] §6 is our shared contract.** Turn it into numbered idempotent
   migrations. Open calls that are yours: (a) the append-only trigger mechanism, (b) whether to
   split `idempotency_keys` into its own table (I recommend not), (c) which evolution satisfies
   R15 (holds vs currency metadata — see §6, coordinate with me if it is holds because it touches
   the `Ledger` interface).

5. **[Kofi — adapter contract] `InTx` MUST use `BEGIN IMMEDIATE` and the DB MUST be opened with
   `journal_mode=WAL`, `busy_timeout` set, and `foreign_keys=ON`.** These are not optional — the
   concurrency correctness in §5 depends on all three. A `SQLITE_BUSY` after the busy_timeout is a
   retryable infra error, surfaced distinct from an invariant failure so R19 can report "zero
   failed invariants".

6. **[Nia — read shapes] The UI consumes api, not core, but the shapes originate here:** a
   `Balance` is `{amount: int64 (minor units), currency}` — render with client-side scaling,
   never parse money as a float. A `Statement` is keyset-paged on `seq` (`Page{Limit, AfterSeq}`
   → `StatementPage{..., NextAfterSeq, HasMore}`); build "load more" on `NextAfterSeq`, not
   offset. A rejected transfer carries the typed `{code, message}` body (R22/R13) — show the
   message, key behaviour off the `code`.

7. **[all — system/mint account] One designated account carries `AllowNegative = true`; it is the
   source of all minted value (R2 via §3). Who provisions it and how its `CreateAccountCmd` is
   authorised (not every caller may set `AllowNegative`) is an api-gating decision — Dakari, this
   is yours, but flagging it here because the engine trusts the flag.**

---

*Frozen by Amara Okeyo, core/architect. §4 and §6 are the chain's load-bearing surfaces —
build to them, and bring changes back to me.*

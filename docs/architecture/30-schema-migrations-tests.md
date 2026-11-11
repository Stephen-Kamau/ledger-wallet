# 30 — Schema, Migrations & Cross-Stream Tests

**Author:** Kofi Otieno (`kofi-1`, builder) — owner of the `migrations/` and `tests/`
streams.
**Consumes:** Amara's frozen contract `docs/architecture/00-core.md` §4 (ports), §5
(concurrency), §6 (schema). §6 is the **shared** surface (R24) — this document is where I
turn her logical schema into numbered, idempotent SQL and record my position on the one
decision she left open to me.
**Module:** `github.com/Stephen-Kamau/ledger-wallet` · **Go:** 1.24 · stdlib + `modernc.org/sqlite`
(or `mattn/go-sqlite3`) behind the adapter; `tests/` use the same driver the adapter uses.
**Status:** DESIGN. Freezes the migration layout, the `0001`/`0002` DDL, and the test shapes.
Downstream code (the `internal/sqlite` adapter) reads the exact columns frozen in §6 of
`00-core.md` and the DDL below.

---

## 1. Migration strategy (R14)

### 1.1 Layout and naming

All schema lives in `migrations/` as numbered, forward-only SQL files:

```
migrations/
├── 0001_init.sql          # Amara's three tables (§6), PRAGMAs, append-only trigger
├── 0002_currencies.sql    # the post-v1 evolution (R15) — see §3
└── migrate.go             # the applier (go:embed), owned by this stream
```

Naming rule, enforced by the applier, not by convention:

- `NNNN_<slug>.sql`, `NNNN` a zero-padded, strictly increasing integer starting at `0001`.
- Lexical sort over the four-digit prefix **is** apply order. No file is ever edited after it
  ships (append-only migrations mirror append-only postings — a correction is a new file,
  never an edit to an applied one). The applier refuses a gap or a duplicate prefix.

### 1.2 The applier — a tiny Go migrator

The applier reads `migrations/*.sql` in prefix order and applies any not yet recorded in a
`schema_migrations` ledger. It is idempotent at two layers (defence in depth): every DDL
statement is `IF NOT EXISTS`, **and** the ledger means an already-applied file is skipped
entirely. Re-running against an up-to-date DB touches nothing and returns no-op.

The ledger table is created by the applier itself (bootstrap), not by `0001`, so the applier
can record `0001` as applied:

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
  version     TEXT PRIMARY KEY,   -- the NNNN prefix, e.g. "0001"
  name        TEXT NOT NULL,      -- full filename, for audit
  applied_at  TEXT NOT NULL       -- RFC3339 UTC
);
```

Applier sketch (illustrative — lives in `migrations/migrate.go`, in my stream):

```go
package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed *.sql
var files embed.FS

// Apply runs every embedded migration not yet recorded, in prefix order, each in its own
// transaction. It is a no-op when the DB is already current. PRAGMAs that cannot run inside
// a transaction (journal_mode) are executed by the SQLite adapter on open, NOT here — see §1.3.
func Apply(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(
		version TEXT PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	applied, err := appliedVersions(db) // SELECT version FROM schema_migrations
	if err != nil {
		return err
	}

	entries, _ := files.ReadDir(".")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // prefix order == apply order

	var lastVer string
	for _, name := range names {
		ver := name[:4] // "0001"
		if lastVer != "" && ver <= lastVer {
			return fmt.Errorf("migrations: non-increasing or duplicate version %q after %q", ver, lastVer)
		}
		lastVer = ver
		if applied[ver] {
			continue // ledger says done — skip (the no-op path)
		}
		body, _ := files.ReadFile(name)

		tx, err := db.Begin()
		if err != nil {
			return err
		}
		// A migration file may hold many statements; the driver must allow multi-statement Exec,
		// or the applier splits on ";" at statement boundaries. modernc.org/sqlite executes the
		// whole script in one Exec.
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrations: %s failed: %w", name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version,name,applied_at) VALUES(?,?,?)`,
			ver, name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
```

Each file applies in a transaction with its ledger row, so a file either lands whole or not at
all — a half-applied migration cannot be recorded as done.

### 1.3 PRAGMA placement — a correctness note, not a detail

Amara's §5 and open-question 5 require `journal_mode=WAL`, `busy_timeout`, `foreign_keys=ON`.
These are **not uniform** in lifetime, and getting this wrong silently breaks the concurrency
model:

| PRAGMA | Scope | Where it is set | Why |
|---|---|---|---|
| `journal_mode=WAL` | **persistent** (DB header) | `0001_init.sql`, outside any txn | Cannot be changed inside a transaction; set once, sticks. |
| `foreign_keys=ON` | **per-connection** | adapter, on **every** `Open` | Resets to OFF on each new connection — a migration setting it is worthless to the adapter. |
| `busy_timeout` | **per-connection** | adapter, on every `Open` | A blocked `BEGIN IMMEDIATE` must wait, not error (§5). |

So `0001` carries only `journal_mode=WAL` (persistent, legitimately schema). `foreign_keys`
and `busy_timeout` belong to the adapter's connection setup — I flag this to whoever owns
`internal/sqlite` so they are not assumed to come from the migration. `journal_mode=WAL` and
the trigger must run **outside** a transaction; the applier runs `0001` in a txn, so I keep the
`PRAGMA journal_mode=WAL` as the first standalone statement and rely on the driver executing it
before the implicit statement grouping — if the driver rejects WAL-set-in-txn, the applier
special-cases `0001`'s PRAGMA to run on the raw connection before the transactional body. (Open
question 6.1 to the adapter owner.)

### 1.4 `0001_init.sql` — Amara's three tables, realised

```sql
-- 0001_init.sql — v1 schema for the double-entry ledger.
-- Implements docs/architecture/00-core.md §6. Idempotent: safe to re-run.

-- Persistent PRAGMA (DB-header, survives across connections). Per-connection PRAGMAs
-- (foreign_keys, busy_timeout) are the adapter's responsibility on each Open — see §1.3.
PRAGMA journal_mode = WAL;

-- §6 accounts: owned balance holder. NO balance column — balance is derived (R10).
CREATE TABLE IF NOT EXISTS accounts (
  id             TEXT    PRIMARY KEY,
  owner          TEXT    NOT NULL,
  currency       TEXT    NOT NULL,
  allow_negative INTEGER NOT NULL DEFAULT 0 CHECK (allow_negative IN (0,1)),
  created_at     TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_accounts_owner ON accounts(owner); -- "my accounts" (R20)

-- §6 transfers: unit of intent AND the idempotency unit (R4). Key folded in (see §2).
CREATE TABLE IF NOT EXISTS transfers (
  id              TEXT    PRIMARY KEY,
  owner           TEXT    NOT NULL,
  idempotency_key TEXT    NOT NULL,
  from_account    TEXT    NOT NULL REFERENCES accounts(id),
  to_account      TEXT    NOT NULL REFERENCES accounts(id),
  amount          INTEGER NOT NULL CHECK (amount > 0), -- positive magnitude, minor units
  currency        TEXT    NOT NULL,
  created_at      TEXT    NOT NULL,
  UNIQUE (owner, idempotency_key)            -- the R4 backstop under contention (§5)
);

-- §6 postings: append-only immutable fact (R5).
CREATE TABLE IF NOT EXISTS postings (
  id          TEXT    PRIMARY KEY,
  transfer_id TEXT    NOT NULL REFERENCES transfers(id),
  account_id  TEXT    NOT NULL REFERENCES accounts(id),
  amount      INTEGER NOT NULL,          -- signed: debit<0, credit>0
  currency    TEXT    NOT NULL,
  seq         INTEGER NOT NULL,          -- monotonic per account (R12)
  created_at  TEXT    NOT NULL
);

-- Amara's §6 asks for INDEX(account_id, seq). I STRENGTHEN it to UNIQUE — see §2, decision 2.
-- Serves the Balance SUM and the Statement keyset page, AND makes a seq collision impossible.
CREATE UNIQUE INDEX IF NOT EXISTS idx_postings_account_seq ON postings(account_id, seq);

-- Append-only enforcement (R5) at the storage boundary — defence in depth beyond the Tx port
-- having no update/delete method. Amara flagged the mechanism as my call (§6); I take the
-- trigger. CREATE TRIGGER IF NOT EXISTS is idempotent.
CREATE TRIGGER IF NOT EXISTS postings_no_update
BEFORE UPDATE ON postings
BEGIN
  SELECT RAISE(ABORT, 'postings are append-only (R5): UPDATE forbidden');
END;

CREATE TRIGGER IF NOT EXISTS postings_no_delete
BEFORE DELETE ON postings
BEGIN
  SELECT RAISE(ABORT, 'postings are append-only (R5): DELETE forbidden');
END;
```

Notes on what I added beyond the bare §6 table and why each earns its keep:

- `CHECK (allow_negative IN (0,1))` — the column is a boolean carried as INTEGER; the check
  stops a stray `2` becoming a silent third state.
- `CHECK (amount > 0)` on `transfers.amount` — §6 says "positive magnitude"; the schema should
  enforce it, not trust the engine. (Postings carry the *signed* amount and get no such check.)
- `idx_accounts_owner` — R20 "a principal sees **their** accounts" is an owner-scoped scan; the
  index is cheap and the read path is real.

These are additive constraints on columns Amara already froze; they do not rename or retype
anything, so they are not a chain event. The one change that touches her stated contract — the
UNIQUE index — I call out explicitly in §2 and §6 for her to ratify.

---

## 2. My position on the shared schema contract (the R24 overlap)

Amara left me three open calls (00-core.md open-question 4). Here is my verdict on each,
recorded honestly as a cross-operator negotiation.

### Decision 1 — idempotency key folded into `transfers`: **ACCEPT. No contest.**

Amara folds the key into `transfers` with `UNIQUE(owner, idempotency_key)` rather than a
standalone `idempotency_keys` table, inviting me to contest if I have a reason to split it
(e.g. recording in-flight keys before the transfer commits).

I do not. Her argument holds and the concurrency model closes the one hole a split would
address:

1. The key has a **1:1 relationship** with a transfer and **no independent lifecycle**. A
   separate table is a second row to keep consistent for zero benefit — and as the person who
   writes the consistency tests, a second row I'd have to prove stays in lockstep is a cost I
   am glad not to own.
2. The "reserve the key before commit" case **does not arise** under §5's single serialized
   writer. The lookup (`FindTransferByKey`) and the insert happen inside one `BEGIN IMMEDIATE`
   transaction holding the write lock. There is no window between "reserve" and "commit" for a
   second actor to see a half-born key, because there is no second concurrent writer. A
   separate in-flight table solves a race the architecture has already foreclosed.
3. `UNIQUE(owner, idempotency_key)` scoped **per owner** is correct — one client's key space is
   its own, and a global unique key would let client A's key collide with client B's. Agreed.

So: **freeze it folded.** I will not build an `idempotency_keys` table.

### Decision 2 — the index: I **strengthen** `INDEX(account_id, seq)` to **UNIQUE**. (Needs Amara's ratify.)

This is the one place I change what §6 literally states, so I flag it loudly. §6 specifies a
plain `INDEX(account_id, seq)`. I make it a `UNIQUE INDEX`.

Reasoning, from the QA seat:

- §6 already asserts `seq` is "monotonic per account". If that invariant holds, a UNIQUE index
  costs nothing — every `(account_id, seq)` pair is already distinct by construction.
- If the invariant ever **fails** — a bug in how the engine computes the next `seq`, a
  concurrent-writer regression — a plain index hides it and the Statement silently
  double-orders or the keyset page skips. A UNIQUE index turns that latent corruption into a
  loud write failure at the moment it happens. That is exactly the kind of defect that an
  append-only ledger must never absorb silently.
- It is a pure strengthening: same columns, same order, same read plan. Nothing that reads the
  index changes. It only removes a failure mode.

This forces a real question back to Amara (open question 6.2): **where is `seq` generated, and
is that generation inside the write transaction?** With the single serialized writer, the only
safe site is inside `InsertTransfer`, computing `next = COALESCE(MAX(seq),0)+1 WHERE
account_id=?` under the held write lock. If the engine ever tries to pre-compute `seq` outside
the lock, the UNIQUE index will catch it — which is the point. I implement UNIQUE and ask her
to confirm the generation site matches.

### Decision 3 — append-only mechanism: I take the **trigger**. (My call per §6. Done.)

Amara recommended a migration-level trigger and left the mechanism to me. I implement the pair
of `BEFORE UPDATE`/`BEFORE DELETE` triggers in `0001` (§1.4). This enforces R5 at the storage
boundary in addition to the `Tx` port having no update/delete method — belt and braces, and it
means a hand-run `UPDATE postings ...` at a `sqlite3` prompt is refused too, not just engine
code paths.

One consequence I own and test: the triggers must **not** fire on INSERT (they don't — scoped
to UPDATE/DELETE), and a legitimate correcting transfer (a new compensating posting) is an
INSERT, so it is unaffected. My test suite asserts both: a direct `UPDATE`/`DELETE` on postings
raises, and a compensating transfer succeeds.

---

## 3. Schema evolution — `0002` (R15, R16)

### 3.1 Choice: the currency-metadata table, not `holds`.

Amara offered two evolutions and was explicit: `holds` would extend the `Ledger` **interface**
(a `Hold`/`Capture` pair), making it a chain event she owns and has "not frozen now"; the
`currency` metadata migration is the simpler one that satisfies R15 **without** touching the
interface.

As the migrations owner wanting an evolution I can author and verify **independently** — without
blocking on a `core` interface change — I take the **currency-metadata table**. It:

- is purely additive (a new table + seed rows), so bootstrap parity (R16) is trivial to prove;
- touches **nothing** in the `Ledger` port, so it is not a chain event and needs no reconcile
  round with Amara;
- still exercises the real thing R15 is here to test: a second operator authoring a second
  migration on top of a frozen v1.

`holds` is the **richer** v2 evolution and I record it as DEFERRED to Amara — trigger: when the
roster wants authorize-now/capture-later semantics, it starts as a `Ledger` extension
(`Hold`/`Capture`), and only then do I add the `holds` table and the "available = Σ postings −
Σ active holds" rule. That order is non-negotiable because it crosses the interface.

### 3.2 `0002_currencies.sql`

```sql
-- 0002_currencies.sql — post-v1 evolution (R15): currency metadata.
-- Additive only: a new table + idempotent seed. No change to accounts/transfers/postings.
-- Adds the scale needed to render minor units correctly in the UI (Nia, R20/R22) without
-- parsing money as float — the scale lives here, not in client code.

CREATE TABLE IF NOT EXISTS currencies (
  code       TEXT    PRIMARY KEY,          -- matches accounts.currency / transfers.currency
  minor_unit INTEGER NOT NULL CHECK (minor_unit >= 0), -- decimal places: 2 => cents, 0 => yen
  symbol     TEXT    NOT NULL,
  created_at TEXT    NOT NULL
);

-- Idempotent seed: INSERT OR IGNORE so re-running 0002 adds nothing. Fixed rows => a fresh DB
-- and an evolved DB land identical content (R16).
INSERT OR IGNORE INTO currencies(code, minor_unit, symbol, created_at) VALUES
  ('USD', 2, '$',  '2026-01-01T00:00:00Z'),
  ('GBP', 2, '£',  '2026-01-01T00:00:00Z'),
  ('EUR', 2, '€',  '2026-01-01T00:00:00Z'),
  ('JPY', 0, '¥',  '2026-01-01T00:00:00Z');
```

Deliberately **no** foreign key from `accounts.currency` → `currencies.code`. SQLite cannot
`ALTER TABLE ... ADD CONSTRAINT`; adding that FK means a 12-step table rebuild (new table, copy,
drop, rename) which is exactly the kind of destructive, non-additive migration that breaks
bootstrap parity and needs explicit human sign-off. The metadata table stands alone; the engine
may consult it for scale, and a future `0003` can do the rebuild if validation-against-the-table
is ever required. Keeping `0002` additive is a correctness choice, not laziness.

### 3.3 Bootstrap parity (R16), stated concretely

There is no separate `bootstrap.sql`: "bootstrap" is the applier running **all** migrations on
an empty DB. Parity means:

> A DB built by `Apply` on an empty file (`0001` then `0002`) is schema-identical and
> seed-identical to a DB that ran `0001`, was used (accounts created, transfers posted), then
> had `0002` applied later.

Because every migration is additive and every seed is `INSERT OR IGNORE` of fixed rows, this
holds by construction. I prove it with a test (§4.4) that compares the **canonical schema** of
the two DBs:

```sql
SELECT type, name, sql FROM sqlite_schema
WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name;
```

plus the `currencies` seed contents and the `schema_migrations` version set. The user-data rows
(accounts/postings in the evolved DB) are expected to differ — parity is a claim about
**schema and seed**, not about transactional data, and I assert exactly that scope.

---

## 4. Concurrency & bootstrap test strategy (R16, R17, R18)

All tests live in `tests/` and run against a **real** SQLite file in WAL mode through the same
adapter the app uses — not a fake `Store`. Amara's fake-`Store` unit tests prove the engine's
logic; my job is to prove the **storage adapter and the concurrency model** hold under real
contention. Different level, deliberately (tests come first, at the level the property lives).

### 4.1 Harness

```go
// tests/harness_test.go (sketch)

// newLedger spins a fresh WAL DB in a temp dir, applies all migrations, and wires the real
// sqlite.Store into core.New. busy_timeout is set HIGH so a blocked BEGIN IMMEDIATE waits
// rather than erroring — a SQLITE_BUSY here would be infra, not an invariant failure (§4.5).
func newLedger(t *testing.T) (core.Ledger, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "ledger.db")+
		"?_pragma=busy_timeout(10000)&_pragma=foreign_keys(ON)")
	must(t, err)
	must(t, migrations.Apply(db))
	return core.New(sqlite.New(db)), db
}

// fund mints `amount` into `acct` from the single AllowNegative system account, so a test
// account starts with real derived balance (R2: minting is a transfer from the system account).
func fund(t *testing.T, l core.Ledger, sys, acct core.AccountID, owner core.PrincipalID, amount core.Amount) { ... }

// barrier releases all goroutines at once to maximise contention on the write lock.
func barrier(n int) (ready *sync.WaitGroup, go_ chan struct{}) { ... }
```

### 4.2 R17 — no lost updates

N goroutines each transfer a fixed amount into one target account from a well-funded source,
released simultaneously. The property: **final balance == Σ of the deltas that were applied**,
with no append silently dropped.

```go
func TestR17_NoLostUpdates(t *testing.T) {
	l, db := newLedger(t)
	sys, src, dst := setupAccounts(t, l)      // src funded with N units, dst at 0
	const N = 200

	ready, start := barrier(N)
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		go func(i int) {
			ready.Done()
			<-start                            // all N hit the write lock together
			_, errs[i] = l.Transfer(ctx, core.TransferCmd{
				Authorizer: ownerOf(src), From: src, To: dst,
				Amount: 1, Currency: "USD",
				IdempotencyKey: fmt.Sprintf("r17-%d", i), // distinct keys: all are real transfers
			})
		}(i)
	}
	ready.Wait(); close(start)
	waitAll()

	applied := countNilClassified(t, errs)     // successes; any SQLITE_BUSY counted as infra, retried
	bal := balance(t, l, dst)
	require.Equal(t, core.Amount(applied), bal) // every applied delta landed, none lost
	assertInvariants(t, db)                     // §4.5
}
```

The contention is genuine: distinct idempotency keys mean every goroutine is a real write
fighting for the same write lock, and the derived balance (`SUM(amount)`) is recomputed from
the posting log, so a lost append would show up as a short balance.

### 4.3 R18 — no double-spend

Fund an account with exactly `K` units, launch `M > K` goroutines each trying to move 1 unit
out, released together. The property: **exactly `K` succeed, `M−K` return
`ErrInsufficientFunds`, and the account ends at 0** — the exact affordable subset, no overdraw.

```go
func TestR18_NoDoubleSpend(t *testing.T) {
	l, db := newLedger(t)
	sys, acct, sink := setupAccounts(t, l)
	const K, M = 50, 200
	fund(t, l, sys, acct, ownerOf(acct), K)

	ready, start := barrier(M)
	results := make([]error, M)
	for i := 0; i < M; i++ {
		go func(i int) {
			ready.Done(); <-start
			_, results[i] = l.Transfer(ctx, core.TransferCmd{
				Authorizer: ownerOf(acct), From: acct, To: sink,
				Amount: 1, Currency: "USD", IdempotencyKey: fmt.Sprintf("r18-%d", i),
			})
		}(i)
	}
	ready.Wait(); close(start); waitAll()

	ok, insufficient := tally(results, core.ErrInsufficientFunds) // all other errors fail the test
	require.Equal(t, K, ok)
	require.Equal(t, M-K, insufficient)
	require.Equal(t, core.Amount(0), balance(t, l, acct)) // drained exactly, never below
	assertInvariants(t, db)
}
```

This is the test that `BEGIN IMMEDIATE` + in-transaction R3 (§5) exists to pass. With a naive
`BEGIN DEFERRED`, two goroutines read the same balance, both decide 1 unit is affordable, and
the account goes negative or a write loses — the test would see `ok > K` or a negative balance.

### 4.4 R4 under contention — the UNIQUE backstop path

Not in Amara's R17/R18 list but load-bearing for the schema I froze: W goroutines replay the
**same** idempotency key at once. Property: **exactly one posting pair is written**, every
caller gets the same `TransferResult`, and the `UNIQUE(owner, idempotency_key)` → replay
conversion is exercised (not just the fast-path lookup).

```go
func TestR4_ConcurrentReplaySameKey(t *testing.T) {
	l, db := newLedger(t); sys, a, b := setupAccounts(t, l); fund(t, l, sys, a, ownerOf(a), 10)
	const W = 32
	key := "same-key"
	// ... W goroutines, identical TransferCmd with IdempotencyKey=key, released together ...
	// Assertions:
	require.Equal(t, 1, countTransfersWithKey(db, ownerOf(a), key)) // one transfer row
	require.Equal(t, 2, countPostingsForTransfer(db, key))          // one debit + one credit, not W*2
	require.Equal(t, core.Amount(9), balance(t, l, a))              // moved once, not W times
	assertInvariants(t, db)
}
```

If the lookup ever misses under the race, the second `InsertTransfer` hits the UNIQUE
constraint; the adapter must convert `SQLITE_CONSTRAINT_UNIQUE` on that index into a replay
rather than a hard error (open question 6.3 — who owns that mapping). This test is what proves
that conversion actually happens.

### 4.5 The invariant oracle — `zero failed invariants`

Every concurrency test ends with one shared assertion helper. This is the "zero failed
invariants" R19 depends on, factored out so it runs everywhere:

```go
func assertInvariants(t *testing.T, db *sql.DB) {
	t.Helper()
	// R2 conservation: the whole ledger sums to zero (mint from system account nets zero).
	require.Equal(t, int64(0), scalar(db, `SELECT COALESCE(SUM(amount),0) FROM postings`))
	// R10: every account's derived balance == SUM of its postings (no stored balance to drift).
	//      (spot-checked against l.Balance for each account.)
	// R3: no normal account is negative.
	require.Equal(t, int64(0), scalar(db, `
		SELECT COUNT(*) FROM accounts a WHERE a.allow_negative = 0
		AND (SELECT COALESCE(SUM(amount),0) FROM postings p WHERE p.account_id=a.id) < 0`))
	// R5/referential: no orphan posting.
	require.Equal(t, int64(0), scalar(db, `
		SELECT COUNT(*) FROM postings p WHERE p.transfer_id NOT IN (SELECT id FROM transfers)`))
	// seq integrity: the UNIQUE index guarantees it, but assert no gap-free requirement is violated
	// by checking distinct seq count == posting count per account.
	require.Equal(t, int64(0), scalar(db, `
		SELECT COUNT(*) FROM (SELECT account_id FROM postings GROUP BY account_id
		HAVING COUNT(DISTINCT seq) <> COUNT(*))`))
}
```

A `SQLITE_BUSY` returned to a caller after the busy_timeout is classified as a **retryable infra
error**, counted separately, and retried — it is never folded into the invariant count. That
distinction is what lets R19 honestly report "zero failed invariants under contention" even if
the single writer made a caller wait.

### 4.6 Bootstrap parity test (R16)

```go
func TestR16_BootstrapParity(t *testing.T) {
	fresh := applyAll(t)                 // empty DB -> 0001, 0002
	evolved := applyOne(t, "0001")       // empty DB -> 0001 only
	runSomeTransfers(t, evolved)         // use it: accounts, postings
	applyOne(t, "0002", evolved)         // later -> 0002
	require.Equal(t, schemaOf(fresh), schemaOf(evolved))         // sqlite_schema DDL identical
	require.Equal(t, seedOf(fresh, "currencies"), seedOf(evolved, "currencies"))
	require.Equal(t, versionsOf(fresh), versionsOf(evolved))     // schema_migrations == {0001,0002}
}
```

### 4.7 Append-only enforcement test (R5)

```go
func TestR5_PostingsAppendOnly(t *testing.T) {
	l, db := newLedger(t); /* ... post one transfer ... */
	_, uerr := db.Exec(`UPDATE postings SET amount = amount + 1`)
	require.Error(t, uerr)                       // trigger raises
	_, derr := db.Exec(`DELETE FROM postings`)
	require.Error(t, derr)                        // trigger raises
	// a compensating correction is an INSERT and MUST still work:
	reverseTransfer(t, l)                         // succeeds
}
```

---

## 5. Throughput baseline (R19)

R19 wants a recorded write number, a recorded read number, and zero failed invariants under
load. I measure both and write the numbers to an artifact the burst can cite — a throughput
*baseline*, not a release SLA.

### 5.1 What I measure and record

- **Write throughput:** transfers committed per second. Drive a fixed count (e.g. 5,000)
  through `l.Transfer` with a configurable concurrency (1, 8, 64 goroutines) so the single-writer
  ceiling (§5) is visible, not hidden. The headline number is at the concurrency where it
  plateaus — that plateau *is* the serialized-writer limit, and reporting it honestly is the
  point (Amara's §5 tradeoff: "write throughput is capped at one transfer at a time").
- **Read throughput:** balance reads per second. Drive concurrent `l.Balance` calls across many
  goroutines while a writer runs — WAL lets readers proceed without blocking the writer (§5), so
  this number should dwarf the write number, matching the url-shorten predecessor's read/write
  skew.
- Record: `op, concurrency, n, wall_ms, ops_per_sec, infra_retries, failed_invariants` to
  `tests/throughput_baseline.txt` (and stdout), one row per configuration.

### 5.2 Shape

```go
func TestR19_ThroughputBaseline(t *testing.T) {
	if testing.Short() { t.Skip("baseline is a load test") }
	l, db := newLedger(t); seedForLoad(t, l)
	rec := newRecorder(t, "throughput_baseline.txt")

	for _, c := range []int{1, 8, 64} {
		w := measure(func() { drive(l, transferOp, /*n=*/5000, c) })
		rec.row("write", c, 5000, w)
	}
	r := measure(func() { drive(l, balanceOp, /*n=*/50000, 64) })
	rec.row("read", 64, 50000, r)

	// The release-gate assertion: whatever the throughput, correctness held.
	assertInvariants(t, db)
	require.Equal(t, 0, rec.totalFailedInvariants())
}
```

`Benchmark*` variants (`BenchmarkTransferWrite`, `BenchmarkBalanceRead`) back the same paths so
`go test -bench` gives allocation/ns numbers too, but the **recorded baseline** is the
`TestR19` table — a benchmark's numbers are not an artifact the burst can diff over time, the
recorded file is.

The number is a baseline, not a claim: it is captured on one machine with one driver and cited
as "write ≈ X tps / read ≈ Y rps at this commit", never as a repo-wide performance guarantee.

---

## 6. Open questions back to Amara / the chain

Ordered by who they block.

1. **[adapter owner — PRAGMA/txn] (§1.3)** Can the chosen driver run `PRAGMA journal_mode=WAL`
   inside the applier's per-file transaction, or must `0001`'s PRAGMA run on the raw connection
   first? And confirm `foreign_keys=ON` + `busy_timeout` are set by the adapter on **every**
   `Open` — they are per-connection and a migration cannot supply them.

2. **[Amara — ratify the UNIQUE index] (§2 decision 2)** I strengthened `INDEX(account_id, seq)`
   to `UNIQUE`. Pure strengthening, same read plan, turns a silent seq corruption into a loud
   write failure. Confirm you accept it on the shared §6 contract.

3. **[Amara — seq generation site] (§2 decision 2)** Where is `Posting.seq` computed? For the
   UNIQUE index and the single-writer model to agree, it must be `COALESCE(MAX(seq),0)+1` for the
   account **inside** `InsertTransfer`, under the write lock. Confirm the engine does not
   pre-compute `seq` outside the transaction.

4. **[Amara / adapter owner — UNIQUE→replay mapping] (§4.4)** The §5 backstop requires the
   adapter to convert `SQLITE_CONSTRAINT_UNIQUE` on `(owner, idempotency_key)` into a replay of
   the prior transfer, not a hard error. Who owns that error mapping — the adapter, or does the
   engine catch a typed `ErrDuplicateKey` from `Tx.InsertTransfer`? My R4-contention test asserts
   the behaviour either way, but I need to know which layer to point the assertion at.

5. **[Amara — holds is yours when it comes] (§3.1)** I chose the currency-metadata evolution for
   `0002` precisely because `holds` touches the `Ledger` interface. When the roster wants
   authorize/capture, it starts as your `Hold`/`Capture` contract extension; I add the `holds`
   table and the "available = Σ postings − Σ active holds" rule only after. Recorded DEFERRED.

6. **[Dakari — system/mint provisioning]** My funding helper (`fund`, §4.1) creates the single
   `AllowNegative` account by calling `core.CreateAccount` directly with `AllowNegative=true`.
   That is fine for tests calling `core`, but confirm the api gate (your open question 7) is the
   only thing stopping an untrusted caller setting that flag — my tests deliberately bypass api,
   and I do not want that bypass mistaken for the real provisioning path.

---

*Authored by Kofi Otieno, migrations/tests. §1.4 (`0001`) and §3.2 (`0002`) are the SQL the
adapter reads; §2 records where I accept and where I strengthen Amara's §6. The UNIQUE index
and the seq-generation question are the two items that need her ratify before `0001` is frozen.*

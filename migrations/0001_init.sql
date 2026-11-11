-- 0001_init.sql -- v1 schema for the double-entry ledger.
-- Implements docs/architecture/00-core.md §6 and 30-schema-migrations-tests §1.4.
-- Idempotent: every object is CREATE ... IF NOT EXISTS, so re-running is a no-op.
-- This DDL is the authoritative schema; internal/sqlite's embeddedSchema mirrors it.

-- Persistent PRAGMA (DB-header, survives across connections). The migrator runs any
-- leading PRAGMA statement OUTSIDE its per-file transaction, because WAL cannot be
-- set inside a transaction (it silently fails to take effect). Per-connection PRAGMAs
-- (foreign_keys, busy_timeout) are the adapter's responsibility on each Open -- see §1.3.
PRAGMA journal_mode = WAL;

-- §6 accounts: owned balance holder. NO balance column -- balance is derived (R10).
CREATE TABLE IF NOT EXISTS accounts (
  id             TEXT    PRIMARY KEY,
  owner          TEXT    NOT NULL,
  currency       TEXT    NOT NULL,
  allow_negative INTEGER NOT NULL DEFAULT 0 CHECK (allow_negative IN (0,1)),
  created_at     TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_accounts_owner ON accounts(owner); -- "my accounts" (R20)

-- §6 transfers: unit of intent AND the idempotency unit (R4). Key folded in (§2).
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

-- §6 postings: append-only immutable fact (R5). amount is signed (debit<0, credit>0).
CREATE TABLE IF NOT EXISTS postings (
  id          TEXT    PRIMARY KEY,
  transfer_id TEXT    NOT NULL REFERENCES transfers(id),
  account_id  TEXT    NOT NULL REFERENCES accounts(id),
  amount      INTEGER NOT NULL,
  currency    TEXT    NOT NULL,
  seq         INTEGER NOT NULL,          -- monotonic per account (R12, D-B1)
  created_at  TEXT    NOT NULL
);

-- §6 asks for INDEX(account_id, seq); strengthened to UNIQUE (D-B1, §2 decision 2).
-- Serves the Balance SUM and the Statement keyset page, AND turns a latent seq
-- collision into a loud write failure at the moment it happens.
CREATE UNIQUE INDEX IF NOT EXISTS idx_postings_account_seq ON postings(account_id, seq);

-- Append-only enforcement (R5) at the storage boundary -- defence in depth beyond the
-- Tx port having no update/delete method. Scoped to UPDATE/DELETE, so INSERT (a normal
-- posting, or a compensating correction) is unaffected.
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

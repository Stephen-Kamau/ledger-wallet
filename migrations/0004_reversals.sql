-- 0004_reversals.sql -- post-v1 evolution: the reversal link record.
-- Additive only: a new table plus its indexes and triggers, no change to accounts,
-- transfers, postings, currencies or holds, so bootstrap parity (R16) holds and this is
-- not a chain event. Idempotent: every object is CREATE ... IF NOT EXISTS, so re-running
-- is a no-op (R14).
--
-- WHAT THIS RECORDS. Postings are append-only (R5): a correction is never an edit, it is
-- a NEW compensating transfer in the opposite direction. That leaves the ledger correct
-- but silent about intent -- nothing distinguishes "bob paid alice back" from "transfer
-- t-7 was reversed". This table is that link: one row per reversed entry, naming the
-- original transfer and the compensating transfer that undid it.
--
-- THE GUARANTEE, AND WHY IT IS HERE AND NOT IN GO. An entry may be reversed AT MOST ONCE,
-- and `UNIQUE (original_transfer_id)` is what enforces it. A Go-side "has this already
-- been reversed?" check is a read followed by a write, and two callers interleave inside
-- that gap -- the exact shape of race R18 forbids. The UNIQUE constraint collapses the
-- check and the write into one atomic act: of N concurrent attempts SQLite lets exactly
-- one INSERT commit and refuses the rest, with no cooperation required from the caller.
-- tests/reversals_test.go proves this against a real WAL database under -race, using a
-- DISTINCT idempotency key per goroutine so the R4 transfer constraint cannot be what
-- saves it.
--
-- CLAIM THEN SETTLE. A compensating transfer has no id until core.Ledger.Transfer has
-- returned, so the link is written in two phases:
--
--   claim   INSERT with reversal_transfer_id NULL. The UNIQUE constraint picks the single
--           winner BEFORE any posting happens -- which is the point. A NOT NULL column
--           would force the link to be written after the posting, leaving the posting
--           itself unguarded, which is precisely the window being closed.
--   settle  UPDATE reversal_transfer_id and settled_at once the compensating transfer has
--           committed.
--
-- STATUS: THIS TABLE IS DELIBERATELY UNWIRED (Orchestrator decision, burst `reversals`).
-- Nothing in core, api, internal or cmd reads or writes it; tests/reversals_test.go is its
-- only consumer. Production reverses an entry by DERIVING the compensating entry's id as
-- `rev_<entryID>`, which puts "at most one reversal per entry" on the transfers PRIMARY
-- KEY inside core.Reverse's single write transaction -- no schema change, and no second
-- schema source to drift.
--
-- It is kept rather than deleted ON PURPOSE. The schemas staying APART is exactly what
-- tests/bootpath_test.go detects drift on: that guard boots the real cmd/ledgerd and
-- fails closed if the server is serving the adapter's embeddedSchema instead of the
-- migrations, which is how collision-v2 shipped the GET /currencies 500. Wiring this
-- table would cost the guard its signal.
--
-- So DO NOT "fix" the unwired table by giving it a writer without first deciding what
-- replaces the boot-path guard. Removing the duplication is a live Orchestrator flag:
-- the repo currently enforces one invariant twice, here and on the transfers PK.

CREATE TABLE IF NOT EXISTS reversals (
  id                   TEXT PRIMARY KEY,
  original_transfer_id TEXT NOT NULL REFERENCES transfers(id), -- the entry being undone
  reversal_transfer_id TEXT REFERENCES transfers(id),          -- NULL while unsettled
  owner                TEXT NOT NULL,                          -- the authorising principal (R7)
  reason               TEXT NOT NULL DEFAULT '',
  created_at           TEXT NOT NULL,                          -- when the claim was taken
  settled_at           TEXT,                                   -- NULL while unsettled

  -- THE no-double-reversal GUARANTEE (R18). One row per original entry, enforced by the
  -- database, not by a Go branch a race can slip through.
  UNIQUE (original_transfer_id),

  -- The two settle columns move together: a row is either an open claim (both NULL) or a
  -- settled reversal (both set). Half-settled is not a representable state.
  CHECK ((reversal_transfer_id IS NULL) = (settled_at IS NULL)),

  -- A transfer cannot reverse itself.
  CHECK (reversal_transfer_id IS NULL OR reversal_transfer_id <> original_transfer_id)
);

-- A compensating transfer settles AT MOST ONE reversal: without this, one correcting
-- transfer could be claimed as the reversal of several different entries, and the link
-- would say something false about the ledger. Partial so that open claims, which all
-- carry NULL, do not collide with each other. (SQLite treats NULLs as distinct in a
-- UNIQUE index anyway; the WHERE clause states the intent rather than relying on it.)
CREATE UNIQUE INDEX IF NOT EXISTS idx_reversals_reversal_transfer
  ON reversals(reversal_transfer_id) WHERE reversal_transfer_id IS NOT NULL;

-- Serves "is this entry reversed, and by what?" -- the lookup the statement view (R12)
-- and any future GET need. UNIQUE(original_transfer_id) already indexes the other
-- direction.
CREATE INDEX IF NOT EXISTS idx_reversals_owner ON reversals(owner);

-- R5 posture applied to the link record. Postings get outright UPDATE/DELETE triggers
-- (0001); a reversal row must stay mutable for exactly one transition -- the settle --
-- so the triggers below forbid every OTHER mutation rather than all of them.

-- The original entry a reversal points at is fixed at claim time. Repointing it would
-- silently move the UNIQUE guarantee onto a different entry.
CREATE TRIGGER IF NOT EXISTS reversals_no_repoint_original
BEFORE UPDATE OF original_transfer_id ON reversals
WHEN NEW.original_transfer_id <> OLD.original_transfer_id
BEGIN
  SELECT RAISE(ABORT, 'reversals.original_transfer_id is immutable once claimed');
END;

-- Settle is a ONE-WAY transition. Once a reversal names its compensating transfer, that
-- link is the record; overwriting it (or clearing it back to NULL, which would free the
-- entry to be reversed a second time) is refused.
CREATE TRIGGER IF NOT EXISTS reversals_settle_once
BEFORE UPDATE OF reversal_transfer_id ON reversals
WHEN OLD.reversal_transfer_id IS NOT NULL
 AND (NEW.reversal_transfer_id IS NULL
      OR NEW.reversal_transfer_id <> OLD.reversal_transfer_id)
BEGIN
  SELECT RAISE(ABORT, 'a settled reversal cannot be repointed or unsettled');
END;

-- A SETTLED reversal is append-only: it describes postings that exist and cannot be
-- unsaid. An UNSETTLED claim is deliberately deletable -- if the compensating transfer
-- fails after the claim commits, the claim must be releasable or that entry could never
-- be reversed again. Deleting the claim is the rollback path, and it is the only DELETE
-- this table allows.
CREATE TRIGGER IF NOT EXISTS reversals_no_delete_settled
BEFORE DELETE ON reversals
WHEN OLD.reversal_transfer_id IS NOT NULL
BEGIN
  SELECT RAISE(ABORT, 'a settled reversal is append-only: DELETE forbidden');
END;

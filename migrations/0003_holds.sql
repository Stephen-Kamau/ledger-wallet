-- 0003_holds.sql -- post-v1 evolution: account holds (fund reservations).
-- Additive only: a new table, no change to accounts/transfers/postings, so bootstrap
-- parity (R16) holds and this is not a chain event. Idempotent: CREATE ... IF NOT
-- EXISTS, so re-running is a no-op (R14). Mirrors internal/sqlite's embeddedSchema.
--
-- A hold reserves funds WITHOUT posting: it reduces an account's AVAILABLE balance
-- (posted balance minus the sum of active holds) while leaving the posted balance
-- (R10) untouched. A hold is released explicitly, which frees the reservation.

CREATE TABLE IF NOT EXISTS holds (
  id              TEXT    PRIMARY KEY,
  account_id      TEXT    NOT NULL REFERENCES accounts(id),
  owner           TEXT    NOT NULL,                 -- the authorising principal (R7)
  idempotency_key TEXT    NOT NULL,
  amount          INTEGER NOT NULL CHECK (amount > 0), -- positive magnitude, minor units
  currency        TEXT    NOT NULL,
  active          INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0,1)),
  created_at      TEXT    NOT NULL,
  released_at     TEXT,                             -- NULL while active
  UNIQUE (owner, idempotency_key)                   -- idempotent PlaceHold, like transfers
);

-- Serves the ActiveHoldsTotal SUM (account_id = ? AND active = 1).
CREATE INDEX IF NOT EXISTS idx_holds_account_active ON holds(account_id, active);

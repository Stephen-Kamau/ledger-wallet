-- 0002_currencies.sql -- post-v1 evolution (R15): currency metadata.
-- Additive only: a new table + idempotent seed. No change to accounts/transfers/postings,
-- so bootstrap parity (R16) is trivial and this is not a chain event (30-schema §3).
-- Carries the minor-unit scale needed to render money correctly in the UI (R20/R22)
-- without parsing money as float.

CREATE TABLE IF NOT EXISTS currencies (
  code       TEXT    PRIMARY KEY,          -- matches accounts.currency / transfers.currency
  minor_unit INTEGER NOT NULL CHECK (minor_unit >= 0), -- decimal places: 2 => cents, 0 => yen
  symbol     TEXT    NOT NULL,
  created_at TEXT    NOT NULL
);

-- Idempotent seed: INSERT OR IGNORE so re-running 0002 adds nothing. Fixed rows mean a
-- fresh DB and a step-by-step-evolved DB land identical seed content (R16).
INSERT OR IGNORE INTO currencies(code, minor_unit, symbol, created_at) VALUES
  ('USD', 2, '$', '2026-01-01T00:00:00Z'),
  ('GBP', 2, '£', '2026-01-01T00:00:00Z'),
  ('EUR', 2, '€', '2026-01-01T00:00:00Z'),
  ('JPY', 0, '¥', '2026-01-01T00:00:00Z');

-- No foreign key from accounts.currency -> currencies.code: SQLite cannot ALTER TABLE
-- ADD CONSTRAINT, so adding it would require a destructive 12-step table rebuild that
-- breaks additivity and bootstrap parity. The metadata table stands alone; a future
-- 0003 can do the rebuild if validation-against-the-table is ever required.

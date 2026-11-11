# ledger-wallet — Requirements

Testable requirements for the double-entry ledger/wallet. Each is phrased so a stream can
write a failing test against it first (AI-gile: tests come first, at the level the property
lives). IDs are stable so streams and the CHARTER can reference them.

## Domain invariants (core — Amara)

- **R1 Balanced postings.** Every transfer produces postings that sum to zero. A transfer
  whose debits and credits do not net to zero is rejected and nothing is written.
- **R2 Conservation.** Across the whole ledger, the sum of all account balances is constant
  under any sequence of valid transfers (it only changes via explicit mint/burn at a
  designated system account).
- **R3 No negative balance.** A transfer that would take a normal (asset) account below zero
  is rejected, unless the account is explicitly flagged as allowed to go negative (e.g. the
  system/mint account).
- **R4 Idempotent transfers.** A transfer carries a client-supplied idempotency key;
  replaying the same key returns the original result and does **not** post twice.
- **R5 Append-only postings.** Postings are never updated or deleted; a correction is a new
  compensating transfer.

## Accounts & auth boundary (core + api — Amara, Dakari)

- **R6 Account ownership.** Each account has an owner principal. An account is created with
  an owner and a currency.
- **R7 Authorised transfer.** A transfer debiting an account must be authorised by that
  account's owner. An unauthorised debit is rejected with a 403-class outcome, before any
  posting is attempted.
- **R8 Single currency per transfer.** All legs of a transfer share one currency; a
  cross-currency transfer is rejected (currency conversion is out of scope for the toy).

## API (api — Dakari)

- **R9 Create account.** `POST /accounts` creates an account (owner, currency), returns its id.
- **R10 Get balance.** `GET /accounts/{id}` returns the account's current balance, derived
  from postings (not a stored mutable field).
- **R11 Create transfer.** `POST /transfers` with (from, to, amount, currency, idempotency
  key) performs a balanced transfer subject to R1–R8.
- **R12 Statement.** `GET /accounts/{id}/statement` returns the ordered postings for an
  account.
- **R13 Typed errors.** Rejections return a typed error body (code + message), distinct per
  invariant violated, never a bare 500.

## Schema & migrations (migrations — Kofi)

- **R14 Numbered migrations.** All schema lives in numbered, idempotent migration files
  (`IF NOT EXISTS`); applied in order; re-running is a no-op.
- **R15 Schema evolution.** The schema evolves at least once after v1 (add a `holds` concept
  or a `currency` column) via a new migration, exercising cross-operator migration authoring.
- **R16 Bootstrap parity.** A fresh database brought up by the migrations matches one evolved
  step by step.

## Concurrency & performance (tests — Kofi)

- **R17 No lost updates.** N concurrent transfers against the same account leave the balance
  exactly correct (sum of applied deltas); no update is lost to a race.
- **R18 No double-spend.** Concurrent transfers that would each individually pass R3 but
  together overdraw the account result in exactly the affordable subset succeeding.
- **R19 Throughput baseline.** A load test records a write (transfer) throughput number and a
  read (balance) throughput number, with zero failed invariants under contention.

## Web (web — Nia)

- **R20 Balances view.** A signed-in principal sees their accounts and balances.
- **R21 Statement view.** Selecting an account shows its statement (R12).
- **R22 Transfer form.** A principal can initiate a transfer; a rejected transfer shows the
  typed error (R13), not a generic failure.

## Governance (all — exercised by the roster)

- **R23 Signed operators.** Every operator building the repo has a distinct signed identity
  enrolled in the roster; claims and verdicts are attributed to it.
- **R24 Gates fire.** Overlap on the shared schema contract (Amara ∩ Kofi) and
  review-independence (Zola reviewing a stream it did not build) are exercised on real
  signed activity, not fixtures.

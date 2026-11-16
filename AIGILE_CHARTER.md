---
project: ledger-wallet
created: 2026-11-11
source: verbal - 2026-11-11 (Orchestrator); project.md + requirement.md
---

# AIGILE_CHARTER.md

## Vision

<!-- Updated when direction shifts. Track changes with dates. -->

- [2026-11-11] ledger-wallet is a small but financially honest double-entry ledger / wallet
  service with a Go backend. It exists as the toy vehicle for exercising multi-user AI-gile:
  a roster of signed operators builds it concurrently so ownership, claim-overlap, and
  review-independence gates fire on real activity. It deliberately stresses what the
  url-shorten toy did not — a real auth boundary, write-heavy concurrency, cross-operator
  schema migrations, and a genuine core → api → web dependency chain.

## Hard Specs

<!-- Fixed. Changes require explicit Orchestrator confirmation.    -->

- [2026-11-11] Backend is Go (1.24+), standard library plus a thin router.
- [2026-11-11] Double-entry integrity is non-negotiable: every transfer posts balanced
  debit/credit legs that net to zero (R1); balances are derived from postings, never a
  stored mutable field (R10); postings are append-only (R5).
- [2026-11-11] Every operator building the repo has a distinct signed identity enrolled in
  the roster; multi_user_enabled is ON (R23).
- [2026-11-11] All schema changes go through numbered, idempotent migration files (R14).

## Constraints

<!-- Fixed unless explicitly revised by Orchestrator.             -->

- [2026-11-11] Toy scope: SQLite (WAL) storage, no real currency rails, no KYC, no
  multi-node clustering, no production auth provider.
- [2026-11-11] The `core` package is storage-agnostic so invariants are testable without a
  database.

## Objectives

<!-- Living. Add, refine, or mark complete.                       -->

- [ ] Architecture produced first, through the AI-gile cycle, before production code.
- [ ] core: accounts, postings, double-entry invariant engine (R1–R6).
- [ ] api: account + transfer endpoints with the auth boundary (R7–R13).
- [ ] migrations: numbered idempotent schema + one evolution step (R14–R16).
- [ ] tests: concurrency correctness + a throughput baseline (R17–R19).
- [ ] web: balances / statement / transfer UI (R20–R22).
- [ ] Governance gates demonstrably fire on real signed activity (R24).

## Discovered Capabilities

> Moved to AIGILE_PLAN/CAPABILITIES.md. CHARTER contains Vision, Hard Specs, Constraints, Objectives, and Success Criteria only.

## Challenged Decisions

> Moved to AIGILE_DECISIONS.md. CHARTER contains Vision, Hard Specs, Constraints, Objectives, and Success Criteria only.

## Deferred

> Moved to AIGILE_PLAN/BACKLOG.md. CHARTER contains Vision, Hard Specs, Constraints, Objectives, and Success Criteria only.

## Under Consideration

> Moved to AIGILE_PLAN/BACKLOG.md. CHARTER contains Vision, Hard Specs, Constraints, Objectives, and Success Criteria only.

## Success Criteria

<!-- Evolving. -->

- [2026-11-11] All R1–R24 requirements covered by a stream or explicitly deferred with a
  trigger.
- [2026-11-11] A concurrency test proves no lost updates and no double-spend under
  contention (R17, R18).
- [2026-11-11] The overlap gate (Amara ∩ Kofi on the schema contract) and review-
  independence gate (Zola) are shown firing on signed operator activity.

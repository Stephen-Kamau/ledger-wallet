# ledger-wallet — Build Report

**Project:** ledger-wallet (double-entry ledger/wallet, Go backend)
**Purpose:** a toy vehicle for exercising **multi-user AI-gile** — a roster of signed operators
building one repo concurrently, so the ownership / overlap / review-independence machinery fires
on real work.
**Date:** 2026-11-11 · **Repo:** github.com/Stephen-Kamau/ledger-wallet (private)
**Branches:** `main` = assembled full item (per-operator authorship); `dev` = integration tree
with all AI-gile artefacts.

---

## 1. What was built

A small but financially honest double-entry ledger. Value moves as **balanced transfers**;
balances are **derived from postings**, never stored. Two AI-gile bursts:

- **architecture-v1** — design-first, produced through the AI-gile cycle before any code.
- **build-v1** — the implementation, five operators building concurrently.

| Package | Owner | What it does |
|---|---|---|
| `core/` | Amara | Domain model, the `Ledger`/`Store` ports, the double-entry invariant engine (R1–R8, idempotency). Storage-agnostic. |
| `internal/sqlite/` | Amara | SQLite adapter: WAL + `BEGIN IMMEDIATE` single serialized writer, `seq` assigned in-transaction, pooled. |
| `cmd/ledgerd/` | Amara | Server entrypoint; owns the lifecycle and mounts the api handler. |
| `api/` | Dakari | HTTP: 5 endpoints, Bearer auth boundary, typed error→status mapping, idempotency. |
| `migrations/` | Kofi | Numbered idempotent migrations (0001 schema, 0002 currencies) + an embed migrator. |
| `tests/` | Kofi | Cross-stream concurrency + throughput suite. |
| `web/` | Nia | React + TypeScript UI (balances, statement, transfer). |
| review | Zola | Independent review of the built diff (built nothing — the point). |

**Design decisions that shaped the build** (`AIGILE_DECISIONS.md`): single serialized writer via
`BEGIN IMMEDIATE` (no lost updates / no double-spend fall out of serialization); `int64` minor
units end to end (no floats); append-only postings enforced by the absence of an update path plus
DB triggers; non-owned reads return **404** (not 403) to avoid leaking account existence.

---

## 2. How it was built — the multi-user process

Each operator has a **distinct signed identity** (Ed25519 key in a local keystore, enrolled in a
root-signed roster) and works in **its own clone** on the shared `dev` branch. `multi_user_enabled`
is ON (quorum-signed). The chain of work respected a real dependency order:

```
Amara (core)  ──freezes the contract──►  Dakari (api)  ──►  Nia (web)
      └────────────────────────────────►  Kofi (migrations + tests)
                                  Zola (independent review)
```

Every operator minted a **cryptographically signed claim** over the files its plan declared, at the
Planning→Producing transition. The claims are what the gates arbitrate:

| Operator | Signed claim (declared scope) |
|---|---|
| amara | `core/**`, `internal/sqlite/**`, `cmd/ledgerd/**` |
| dakari | `api/**`, `cmd/ledgerd/**` |
| kofi | `migrations/**`, `tests/**` |
| nia | `web/**` |

Full timestamped command-by-command trail: **`docs/operators-activity-log.md`**.

---

## 3. Issues realised during the build

Five distinct issues surfaced — four caught by the process itself, one a weakness in how the
coordinator integrated.

### 3.1 The deliberate conflict: an operator took unassigned work
`cmd/ledgerd/` was Amara's (decision D-A1). **Dakari grabbed it** — he edited `cmd/ledgerd/main.go`
to mount his router, because he needed a running server. This was intentional (we wanted to
exercise contention).

- **Detection (real, not simulated):** `ag-plan-overlap` and `ag_claims.relation` flagged
  **`amara ∩ dakari` on `cmd/ledgerd/**`**. Kofi was cleanly disjoint from everyone.
- **Adjudication:** the owner (Amara) ruled. She **rejected the grab's shape** — mounting
  `ListenAndServe` inside her entrypoint inverted the dependency direction — kept ownership, and
  folded in Dakari's `api.NewHandler` herself. Dakari's edit was reverted; his retained
  contribution is `api/` only.
- **Verification:** Zola confirmed via `go list` that no `api→cmd` or `core→cmd` edge exists.

This is the headline result: the multi-user machinery turned an unassigned edit into a flagged,
owner-adjudicated decision rather than a silent clobber.

### 3.2 Adapter never capped its connection pool (found by Kofi)
Amara's `internal/sqlite.Open` **commented** that it capped the pool but never called
`SetMaxOpenConns`. Under read fan-out the uncapped `database/sql` pool churned fresh connections
(re-running DSN PRAGMAs each time), collapsing reads to ~193 rps at concurrency 64. Kofi found it
running the throughput suite; **Amara fixed it in the same burst** (cap 16 + a regression test).

### 3.3 `seq` not written back into the transfer result — C-1 (found by Zola, review-independence earning its keep)
The adapter computed `postings.seq` (`MAX(seq)+1` under the write lock) and INSERTed it, but ranged
over the postings **by value**, so the returned result reported `seq: 0`. **The in-memory fakes did
write seq back, which hid the divergence** — classic test theater on exactly the field decision
D-B1 was about. Only an independent reviewer with no stake in the code caught it. Fixed in-burst
with a real-adapter test the fakes cannot satisfy.

### 3.4 Throughput contradicted the design claim — C-2
The architecture predicted "reads dwarf writes" under WAL. The measured R19 baseline shows the
**opposite**: reads ~5× slower than writes. Causes: a ~0.6ms/query floor in the pure-Go SQLite
driver, the derived-balance read being an O(postings) `SUM`, and a two-query read path. Not a
correctness bug — the design doc was reconciled to the empirical reality so the papers don't assert
a false number.

### 3.5 Attribution weakness in the first integration (process, not product)
The coordinator initially squashed every operator's work into coordinator-authored commits on `dev`,
so `git blame` on `dev` could not tell you who wrote what. Corrected: each operator's work was
rebuilt as a branch authored by them (`feature/amara-core`, `-kofi-migrations`, `-dakari-api`,
`-nia-web`), opened as four PRs, and merged into `main` — so authorship is now in git history.

---

## 4. Verification results

Everything below was run and observed, not asserted.

| Layer | Result |
|---|---|
| `go build ./...`, `go vet ./...` | clean |
| `go test ./...` (main) | **all green** — `core`, `internal/sqlite`, `api`, `tests` |
| `go test -race ./tests/` | green |
| Concurrency R17 (no lost updates) | 200 distinct transfers; final balance == Σ deltas |
| Concurrency R18 (no double-spend) | fund 50, fire 200 withdrawals → exactly 50 succeed, 150 `insufficient_funds`, drains to 0, never negative |
| R4 under contention | 32 goroutines, same key → 1 transfer, 2 postings, balance moves once |
| R16 bootstrap parity | fresh DB == step-by-step evolved DB |
| Web | `tsc --noEmit` clean (strict), `vitest` 23/23, `vite build` clean |
| **End-to-end** (`ledgerd` running) | mint, transfer, idempotent replay (no double-post), insufficient-funds 409, auth boundary 403 on debiting another's account, **conservation sum = 0** |

### Measured throughput (modernc pure-Go SQLite, single process, localhost)

| op | c=1 | c=8 | c=64 |
|---|---|---|---|
| write (transfer) tps | ~860–3166 | ~611 | ~412 |
| read (balance) rps | ~136–583 | ~189–266 | ~238 |

The write number degrading with concurrency is the expected serialized-`BEGIN IMMEDIATE` ceiling,
reported honestly rather than hidden.

---

## 5. Observations

**What the multi-user machinery actually demonstrated**
- **Signed claims are real attribution.** The claim signature is keyed to the operator's private
  key; it is stronger evidence of "who claimed this scope" than a git author line (which is
  self-declared). The conflict and the review-independence verdicts both ran off these claims.
- **The overlap gate caught the grab, the owner resolved it.** Not a merge conflict after the fact —
  the contention was surfaced at claim time and adjudicated by the file's owner with a design
  rationale, and the resolution was independently verified.
- **Review-independence is enforceable, not advisory.** `ag_claims.check_independence` returned
  **zola = PASS** (no claim) and **amara/dakari = BLOCK** (they built the code). Only the
  non-builder could sign off — and that independence is exactly what caught C-1, the bug the
  builders' own fakes concealed.
- **Defects found in the burst were fixed in the burst** (pool cap, C-1), per AI-gile discipline,
  rather than filed for later.

**Honest limitations**
- **Plan-store state is per-clone, not merged.** The coordinator integrates *code + signed claims*
  into the shared tree, not each operator's AI-gile plan-store/STATUS. Merging that across operators
  is a known, deliberately-deferred concurrency problem in the methodology (canonical AGV-328/329).
  The claims, the overlap gate, and review-independence are genuine; the plan-store merge is not
  attempted.
- **The conflict was staged.** We instructed one operator to take unassigned work. It is a faithful
  exercise of the mechanism, but it did not arise spontaneously.
- **Throughput is toy-grade** (pure-Go driver, single process). The numbers are for relative
  behaviour under contention, not a capacity claim.
- **`BEGIN IMMEDIATE` backstop (C-3)** — the `UNIQUE→replay` conversion is unimplemented and
  near-unreachable under the serialized writer; deferred with a trigger (revisit if the writer model
  is relaxed).

**Bottom line.** The product is a correct, tested double-entry ledger. More importantly for the
exercise: the multi-user substrate fired on real signed work — it detected an unassigned-work
conflict, let the owner adjudicate it, enforced that only an independent party could review, and
that independent review caught a real bug the builders had hidden from themselves. The process did
its job.

---

## 6. Where to look

- `docs/operators-activity-log.md` — timestamped per-operator command/issue/conflict trail
- `docs/operators-review-build-v1.md` — Zola's independent review
- `docs/architecture/*.md` — the frozen design + its independent review
- `AIGILE_DECISIONS.md` — conditions A–D and the C-1/C-2/C-3 dispositions
- PRs #1–#4 — per-operator contributions, authored by each operator

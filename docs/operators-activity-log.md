# Operators activity log

Running, timestamped record of the multi-user build. Purpose: after the fact, analyse **what
happened, when, by which operator, and whether we hit conflict**. The coordinator (driver)
appends here as the burst runs. Times are UTC.

Columns: **Time (UTC)** | **Operator** | **Action** | **AI-gile / git command** | **Outcome / notes**

## Conventions
- "Operator" is the signed identity acting (git `user.email` local-part = login; claims signed
  with that login's keystore key).
- Conflict rows are marked **⚠ CONFLICT**.
- Decisions carry a `CD-` / condition tag.

## Burst: build-v1

| Time (UTC) | Operator | Action | Command | Outcome / notes |
|---|---|---|---|---|
| 2026-11-11T18:27Z | coordinator | audit log created | (Write) | this file |
| 2026-11-11T18:28Z | coordinator | resolve conditions A-D | (AIGILE_DECISIONS.md D-A1..D-D1) | A: internal/sqlite+cmd/ledgerd -> Amara; B: seq in adapter + UNIQUE; C: AccountsByOwner + GET /accounts; D: read 404 on non-owned |
| 2026-11-11T18:28Z | coordinator | start build burst | `ag-phase transition <main> Planning` + plan-store write plan | burst build-v1, union plan committed ba50a5b |
| 2026-11-11T18:29Z | amara | provision clone | `git clone`; `git config user.email amara@ledger.test`; copy .aigile key+flag | operators/amara, multi_user ON, amara key only |
| 2026-11-11T18:30Z | amara | scope + claim | `ag-phase transition <amara> Planning` then `Producing` (capture-base) | **signed claim** base c0ac79d, globs [cmd/ledgerd/**, core/**, internal/sqlite/**], sig present (amara key) |
| 2026-11-11T18:41Z | amara | code core+adapter+ledgerd | (delegated agent) `go test ./...` | 17 tests pass, -race clean, modernc SQLite; committed 201de13 |
| 2026-11-11T18:43Z | coordinator | integrate Amara core -> dev | `git fetch <amara> dev`; `git checkout -- core internal cmd` + copy claim | main builds+tests green; pushed to github |
| 2026-11-11T18:45Z | dakari | provision + scope + claim | `git clone`; `ag-phase ...Producing` | **signed claim** base fec52f7, globs [api/**, **cmd/ledgerd/**] <- GRAB], sig yes |
| 2026-11-11T18:45Z | kofi | provision + scope + claim | `git clone`; `ag-phase ...Producing` | **signed claim** base 893749a, globs [migrations/**, tests/**], sig yes |
| 2026-11-11T21:46Z | dakari | code api/ + GRAB cmd/ledgerd/main.go | (delegated agent) `go test ./api/` | 28 api tests pass; edited `cmd/ledgerd/main.go` (Amara's file) to mount router |
| 2026-11-11T21:51Z | kofi | code migrations/ + tests/ | (delegated agent) `go test ./tests/` | `ok tests 27.0s`, 0 failed invariants; found adapter defect: `sqlite.Open` never calls SetMaxOpenConns (pool-cap claim unbacked) |
| 2026-11-11T21:55Z | coordinator | **overlap gate** | `ag-plan-overlap`; `ag_claims.relation` | **⚠ CONFLICT amara ∩ dakari on `cmd/ledgerd/**`** (shared declared glob; Dakari's real diff touches `cmd/ledgerd/main.go`). amara∩kofi, dakari∩kofi: clean (disjoint). 3 WARNs, gate never blocks (plan-time guard). |
| 2026-11-11T21:56Z | coordinator | **conflict adjudication** | (ownership clock, D-A1) | `cmd/ledgerd` ownership STAYS Amara (architect owns server wiring). Dakari's grab is accepted only after Amara (owner) reviews+folds the change. Unassigned-take recorded against dakari. |
| 2026-11-11T22:05Z | coordinator | integrate api+migrations+tests -> dev | `git fetch`/`checkout`; `go test ./...` | full backend green (api, core, sqlite, tests); pushed db59b7c |
| 2026-11-11T22:08Z | amara | fix adapter defect | (resume) `go test ./internal/sqlite/` | SetMaxOpenConns cap=16 + idle + TestOpen_PoolCapped; 7 tests green. (defect: kofi) |
| 2026-11-11T22:08Z | amara | **ownership ruling on grab** | (owner decision D-A1) | REJECTS Dakari's shape (ListenAndServe in her main.go). Ruling: cmd/ledgerd stays Amara; api exposes NewHandler; Amara folds the mount, Dakari's edit reverted. cmd<-api<-core preserved. |
| 2026-11-11T22:10Z | coordinator | **resolve conflict** | rewrite cmd/ledgerd/main.go (Amara-authored) | grab reverted, owner's wiring applied; build+vet+tests green; committed 93cbd7f |
| 2026-11-11T22:11Z | nia | provision + scope + claim | `git clone`; `ag-phase ...Producing` | **signed claim** globs [web/**], sig yes; coding web UI |
| 2026-11-11T22:20Z | nia | code web/ | (delegated agent) `tsc --noEmit`, `vitest run`, `vite build` | tsc clean, vitest 23/23, build clean; committed, integrated to dev |
| 2026-11-11T22:24Z | coordinator | **review-independence gate** | `ag_claims.check_independence` | **zola=PASS** (no claim); **amara=BLOCK**, **dakari=BLOCK** (hold intersecting claims). Only the non-builder may review - gate enforces it. |
| 2026-11-11T22:25Z | zola | independent review | (reviewer seat, read-only) | reviewing full built diff; writing docs/operators-review-build-v1.md |
| 2026-11-11T22:40Z | zola | review verdict | `go build/vet/test -race` | **APPROVE-WITH-CONDITIONS**. C-1 (real bug: seq not in result, fakes hid it), C-2 (R19 contradicts design claim), C-3 (replay backstop, low). Conflict resolution verified sound (go list). |
| 2026-11-11T22:42Z | coordinator | dispositions | (AIGILE_DECISIONS + DESIGN) | C-1 -> fix in burst (amara); C-2 -> doc reconciled; C-3 -> DEFERRED w/ trigger |
| 2026-11-11T22:45Z | amara | fix C-1 | (resume) `go test ./internal/sqlite/` | seq write-back + real-adapter test fakes can't satisfy; 8 tests pass |
| 2026-11-11T22:47Z | coordinator | integrate C-1 + full suite | `go test ./...` | ALL GREEN: api, core, sqlite, tests (R17/R18/R4/R16/R19). Burst build-v1 done. |
| 2026-11-11T23:10Z | coordinator | attribution fix (per-operator branches) | `git branch`/`checkout dev -- <scope>`/`push`; `gh pr create` | Weakness corrected: dev commits were all coordinator-authored. Built feature/<op> branches off build-base, each authored by the operator (Amara/Kofi/Dakari/Nia), stacked on the dependency chain. 4 PRs opened (#1 amara->build-base, #2 kofi->amara, #3 dakari->amara, #4 nia->dakari). Now git-blameable per operator. |

## Burst: collision-v2 (holds + currencies — same-file collision test)

| Time (UTC) | Operator | Action | Command | Outcome / notes |
|---|---|---|---|---|
| 2026-11-16T07:20Z | coordinator | start burst, overlapping scope | `ag-phase ...`; plan-store | amara(holds)+kofi(currencies) BOTH claim core/**, api/**, internal/sqlite/** |
| 2026-11-16T07:25Z | amara, kofi | signed claims (overlapping) | `ag-phase ...Producing` | **overlap gate flags amara ∩ kofi** on [api/**, core/**, internal/sqlite/**] at CLAIM time |
| 2026-11-16T07:40Z | amara | build holds | (agent) `go test ./...` | PlaceHold/ReleaseHold/AvailableBalance + migration 0003; green; edits core/ledger.go end-of-interface |
| 2026-11-16T07:40Z | kofi | build currencies | (agent) `go test ./...` | ListCurrencies + GET /currencies; green; edits SAME core/ledger.go end-of-interface |
| 2026-11-16T07:45Z | coordinator | **push collision** | amara push -> ff OK; `git push github HEAD:dev` (kofi) | **kofi push REJECTED (non-fast-forward)** — git prevents the clobber (hard enforcement, beyond the advisory overlap warning) |
| 2026-11-16T07:46Z | kofi | **merge dev** | `git merge github/dev` | **CONFLICT in 7 files** (core/ledger.go, core/store.go, api/handler.go, api/handlers.go, api/types.go, both fakes) |
| 2026-11-16T07:50Z | kofi | **resolve keeping BOTH** | (agent) resolve + `go test ./...` | both features survive; a real semantic hazard fixed (two handler bodies were INTERLEAVED at a shared `if err` block); merge committed 7d11fcf; green |
| 2026-11-16T07:55Z | coordinator | integrate both -> dev | `git fetch`/`checkout`; `go test ./...` | dev has PlaceHold + ListCurrencies; full suite green |
| 2026-11-16T07:58Z | coordinator | **e2e on live server** | run ledgerd, curl | holds OK (201/409/204); **GET /currencies 500** — embeddedSchema↔migrations DRIFT: currencies table only in migration 0002, live server boots embeddedSchema. Tests passed, e2e caught it. Routed to owner Amara to fix. |

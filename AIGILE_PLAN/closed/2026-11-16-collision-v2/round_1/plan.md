<!-- AIGILE-RECORD
kind: plan
stage: plan
round: 1
schema_version: 1.1.0
superseded_by: none
-->

# Plan - burst collision-v2 (holds + currencies; same-file collision test)

Two remaining features that deliberately share files so two operators collide on
core/ledger.go + core/store.go + internal/sqlite/store.go. Both edits MUST land.

### S-holds - Amara
- Scope (files WRITTEN):
  - `core/**`
  - `internal/sqlite/**`
  - `migrations/**`
  - `api/**`
### S-currencies - Kofi
- Scope (files WRITTEN):
  - `core/**`
  - `internal/sqlite/**`
  - `api/**`

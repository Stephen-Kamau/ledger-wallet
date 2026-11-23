<!-- AIGILE-RECORD
kind: plan
stage: plan
round: 1
schema_version: 1.1.0
superseded_by: none
-->

# Burst `reversals` - stream plan (dakari)

Single-operator stream plan. This clone declares ONLY dakari's scope; the other four
streams' plans live in their own clones so the signed claims partition cleanly.

## Stream: api - entry reversal endpoint

- Owner: dakari (Dakari Mwangi), rung builder
- Scope (files WRITTEN):
  - `api/**`

### Goal

Expose reversal over HTTP. `GET /accounts/{id}/statement` (R12) ALREADY EXISTS
(`api/handler.go:88`, `getStatement`, keyset-paged) and is NOT rebuilt.

What is missing:

- `POST /entries/{id}/reverse` - Bearer auth, honours the client `Idempotency-Key`
  header, returns the compensating entry.
- A typed `409` when the entry is already reversed, mapped through the single
  fail-closed `coreErrorMap` in `api/errors.go` (R13). No bare 500 for a known rule.
- The read-side auth boundary is settled by **D-D1** in `AIGILE_DECISIONS.md`
  (a read of a non-owned account is 404, not 403, to avoid leaking existence).
  That ruling is followed, not re-litigated.

### Consumed, not owned

`core.Ledger.Reverse` is amara's. `core/**` is OUT of this stream's scope and is not
edited here. This stream codes against the method signature; if it lands different,
the rebase takes amara's and this stream adapts.

### Not in scope

- `core/**`, `internal/**`, `cmd/**`, `web/**`, migrations.
- Rebuilding R12 statement paging.

### Verification

- `go test ./api/...` and `go vet ./api/...` green before the push.
- New table-driven handler tests over the existing `fakeLedger` seam, covering:
  happy path, replay (idempotency), already-reversed 409, unknown entry 404,
  non-owner under D-D1, and missing `Idempotency-Key` 400.
- `TestErrorMap_EveryCoreSentinelMapped` extended with the new core sentinels so the
  fail-closed contract still holds.

# 10 — API Architecture (the HTTP boundary)

**Author:** Dakari Mwangi (`dakari-1`, builder) — owner of the `api/` stream, SECOND link in
the `core → api → web` chain.
**Status:** Design doc. Builds strictly against Amara's FROZEN `Ledger` contract
(`00-core.md` §4). I do not redesign core; where I think the contract blocks the API I FLAG it
in §6, I do not fork it.
**Consumes:** `core.Ledger`, `core.TransferCmd`, the typed `LedgerError` codes (§4, §7).
**Serves:** Nia (`web/`) — the request/response shapes and the error→status table below are the
contract her UI codes to.
**Module:** `github.com/Stephen-Kamau/ledger-wallet` · **Go 1.24** · stdlib `net/http` + a thin
router, no web framework (per Amara: stdlib only).

This is the HTTP side of the auth split Amara named: **api authenticates and sets
`Authorizer`; core authorizes ownership.** api does transport and identity; nothing financial
happens outside `core.Ledger`.

---

## 1. Endpoint design (R9–R12)

Four endpoints, each a thin translation between JSON and one `Ledger` method. The handler's
only jobs: authenticate → decode → populate the core command → call the method → encode the
result or map the typed error (§3). No business logic lives here — that is the whole point of
the seam.

### Endpoint → `Ledger` method table (the load-bearing summary)

| Method & path | `Ledger` method | Success | Auth required |
|---|---|---|---|
| `POST /accounts` | `CreateAccount(ctx, CreateAccountCmd)` | `201 Created` | yes |
| `GET /accounts/{id}` | `Balance(ctx, AccountID)` | `200 OK` | yes |
| `POST /transfers` | `Transfer(ctx, TransferCmd)` | `201` first / `200` replay | yes |
| `GET /accounts/{id}/statement` | `Statement(ctx, AccountID, Page)` | `200 OK` | yes |

Money is **`int64` minor units** on the wire, never a float, never a decimal string (Amara §2,
§7.6). `currency` is an opaque label echoed through. All timestamps are RFC3339 UTC.

### 1.1 `POST /accounts` (R9) → `Ledger.CreateAccount`

Request:
```json
{ "currency": "USD", "allow_negative": false }
```
`owner` is NOT in the body. The owner is the **authenticated principal** (§2) — a client cannot
create an account owned by someone else. `allow_negative` is accepted only from an admin
principal; see §2.4 (system/mint gating, Amara §7.7, my call).

Response `201`:
```json
{ "id": "acc_01H...", "owner": "alice", "currency": "USD",
  "allow_negative": false, "created_at": "2026-11-11T12:00:00Z" }
```

Handler populates:
```go
cmd := core.CreateAccountCmd{
    Owner:         principal,          // from auth, NOT the body
    Currency:      core.Currency(body.Currency),
    AllowNegative: body.AllowNegative, // rejected pre-core with 403 unless principal is admin
}
acc, err := h.ledger.CreateAccount(r.Context(), cmd)
```

### 1.2 `GET /accounts/{id}` (R10) → `Ledger.Balance`

Returns the **derived** balance (Σ postings), never a stored field. Response `200`:
```json
{ "account_id": "acc_01H...", "amount": 42000, "currency": "USD" }
```
`amount` is minor units (42000 = 420.00 USD). Nia scales client-side.

Note the contract shape: `Ledger.Balance` returns `(Amount, error)` only — it does not return
the currency. The API needs the currency for the body, so the handler does one extra read. See
§6 open question (1) — this is the one place the frozen contract makes me do a second call.

### 1.3 `POST /transfers` (R11) → `Ledger.Transfer`

Request:
```json
{ "from": "acc_01H...A", "to": "acc_01H...B",
  "amount": 2500, "currency": "USD" }
```
The idempotency key is a **header**, not a body field — see §5. `from`'s owner must be the
authenticated principal, but the API does NOT check that; it sets `Authorizer` and lets core
reject (§2.2).

Response `201` (first write) / `200` (replay):
```json
{ "transfer_id": "txf_01H...", "from": "acc_...A", "to": "acc_...B",
  "amount": 2500, "currency": "USD", "replayed": false,
  "created_at": "2026-11-11T12:00:01Z",
  "postings": [
    { "account_id": "acc_...A", "amount": -2500, "seq": 7 },
    { "account_id": "acc_...B", "amount":  2500, "seq": 4 }
  ] }
```

Handler populates `TransferCmd` — note `Authorizer` comes from auth, every other field from the
body/header:
```go
cmd := core.TransferCmd{
    Authorizer:     principal,                          // AUTHENTICATED caller (§2)
    From:           core.AccountID(body.From),
    To:             core.AccountID(body.To),
    Amount:         core.Amount(body.Amount),           // positive magnitude From→To
    Currency:       core.Currency(body.Currency),
    IdempotencyKey: r.Header.Get("Idempotency-Key"),    // required (§5)
}
res, err := h.ledger.Transfer(r.Context(), cmd)
// res.Replayed selects 200 vs 201; res.Transfer is the body
```

### 1.4 `GET /accounts/{id}/statement` (R12) → `Ledger.Statement`, keyset-paged

Query params map directly onto Amara's `Page{Limit, AfterSeq}`:
```
GET /accounts/acc_01H.../statement?limit=50&after_seq=0
```
- `limit` → `Page.Limit` (default 50, clamp to a max of e.g. 200 to bound response size).
- `after_seq` → `Page.AfterSeq` (keyset cursor on `Posting.Seq`; default 0 = from the start).

Response `200`:
```json
{ "account_id": "acc_01H...",
  "postings": [
    { "id": "pst_...", "transfer_id": "txf_...", "amount": -2500,
      "currency": "USD", "seq": 7, "created_at": "2026-11-11T12:00:01Z" }
  ],
  "next_after_seq": 7,
  "has_more": true }
```
Keyset, not offset: Nia builds "load more" by passing the returned `next_after_seq` back as
`after_seq` (Amara §7.6). No `OFFSET` anywhere — the `INDEX(account_id, seq)` serves it.

---

## 2. The auth boundary (R7)

This is the half of Amara's split that I own. **Confirmed: I am good with the boundary in
`00-core.md` §7.1.** api proves *who is asking*; core decides *whether they may*.

### 2.1 Chosen mechanism — bearer token carrying the principal

**Choice: `Authorization: Bearer <token>`, where the toy token's subject is the `PrincipalID`.**

Justification over the alternative (`X-Principal: alice` plain header):

- A plain `X-Principal` header is *unauthenticated identity* — any caller claims to be anyone by
  setting a header. For a system whose entire reason to exist is an auth boundary on money
  (project.md point 1), shipping identity as an unsigned header would make R7 untestable in any
  honest way: core would faithfully check `from.Owner == Authorizer`, but `Authorizer` would be
  a self-declared string. The gate would "pass" while controlling nothing.
- Bearer keeps the toy small but real: a token the caller must *present*, that the API
  *validates* before it will set `Authorizer`. The validation can be trivial for the toy (a
  signed token, or a lookup in an in-memory principal→token map seeded at startup) — what matters
  architecturally is that **an invalid/absent token is 401 and never reaches core**.

Toy scope (explicit, so Nia and Kofi know the edges): no refresh, no expiry rotation, no OAuth
provider (project.md "out of scope: production auth providers"). A DEFERRED item: swap the toy
token verifier for a real JWT/OIDC verifier — the `authenticate()` seam below is the single
place that changes.

### 2.2 api does NOT decide ownership

The middleware resolves the token to a proven `PrincipalID` and nothing more. It does **not**
load the account, does **not** compare owners, does **not** pre-check R7. It sets
`TransferCmd.Authorizer = principal` and calls `Ledger.Transfer`. Ownership data lives in core;
the ownership check lives in core (Amara §7.1). The API's contract to core is exactly:
*`Authorizer` is the authenticated subject, never anything the body asked for.*

```go
// in the transfer handler, principal came from the auth middleware's context value:
cmd.Authorizer = principal   // the ONLY thing the API asserts about identity
```

### 2.3 The 403 path

When `from.Owner != cmd.Authorizer`, core returns `ErrUnauthorized`. The API maps that to **403
Forbidden** with code `unauthorized` (§3). The distinction the status table encodes:

- **401 Unauthorized** — the API could not establish *who* is asking (missing/invalid token).
  Decided in middleware, before core.
- **403 Forbidden** — we know who is asking, and core says they do not own the debited account.
  Decided by core, surfaced by the API.

(The HTTP spec's names are famously backwards here; the codes `unauthenticated`-ish=401 /
`unauthorized`=403 follow Amara's frozen `Code` string `unauthorized` for the core error.)

### 2.4 System/mint gating (Amara §7.7 — my call)

`CreateAccountCmd.AllowNegative = true` provisions the one account that may go negative, i.e. the
source of all minted value. Letting any caller set it would let anyone mint money, defeating R2.
**Decision: `allow_negative: true` is accepted only when the authenticated principal is in a
configured admin set** (a startup-config list of principals; empty by default so a fresh toy can
provision its first system account via config/seed, not an open endpoint). A non-admin sending
`allow_negative: true` gets **403** `forbidden_privileged_flag` — rejected at the API, before
core, because it is an identity/privilege decision the API owns, not an invariant core owns. Core
still trusts the flag (Amara §7.7); the API is the gate that keeps the flag honest.

---

## 3. Typed error → HTTP status mapping (R13)

**This table is the contract Nia's UI consumes.** Core returns typed `LedgerError` values with
frozen `Code` strings (Amara §4, §7.2); the API owns the HTTP status (Amara §7.2 handed me this).
I adopt Amara's proposed mapping and extend it to cover the transport-layer and auth-layer cases
core never sees. **Never a bare 500 for a known rule** (R13).

| Core error / condition | HTTP status | `code` (stable) | Decided by | Notes |
|---|---|---|---|---|
| `ErrUnbalanced` | 422 | `unbalanced` | core | legs do not net to zero (R1) |
| `ErrCurrencyMismatch` | 422 | `currency_mismatch` | core | cross-currency transfer (R8) |
| `ErrInsufficientFunds` | 409 | `insufficient_funds` | core | would overdraw (R3); a state conflict, retryable after funding |
| `ErrUnauthorized` | 403 | `unauthorized` | core | caller is not the debited account's owner (R7) |
| `ErrAccountNotFound` | 404 | `account_not_found` | core | unknown `from`/`to`/`{id}` |
| `ErrIdempotencyConflict` | 409 | `idempotency_conflict` | core | same key, different params (R4) |
| **replay** (`Replayed==true`) | **200** | — (success) | core→api | NOT an error; original result (§5) |
| invalid/absent bearer token | 401 | `unauthenticated` | api (middleware) | identity not established (§2.3) |
| non-admin set `allow_negative` | 403 | `forbidden_privileged_flag` | api | privilege gate (§2.4) |
| malformed JSON / wrong types | 400 | `bad_request` | api (decode) | body did not parse |
| missing `Idempotency-Key` on transfer | 400 | `idempotency_key_required` | api | required header absent (§5) |
| missing required body field | 422 | `unprocessable_entity` | api | parsed but semantically incomplete |
| unknown route | 404 | `not_found` | api (router) | — |
| method not allowed on known path | 405 | `method_not_allowed` | api (router) | e.g. `DELETE /accounts` |
| context cancelled / client gone | 499 | `client_closed_request` | api | non-standard but honest |
| `SQLITE_BUSY` after busy_timeout | 503 | `retry_later` | api (from infra err) | retryable infra, NOT invariant failure (Amara §5, §7.5); `Retry-After` header |
| anything unmapped | 500 | `internal` | api (fallback) | logged with request-id; body still typed, never bare |

### Error body shape (one shape, every error)

```json
{ "error": { "code": "insufficient_funds",
             "message": "transfer would overdraw account acc_...A",
             "request_id": "req_01H..." } }
```
Nia keys behaviour off `code` (stable, frozen), shows `message` to the user (R22), and can quote
`request_id` in a support/debug flow. The body shape is identical for a 400, a 409 and a 500 — a
client never has to branch on status to find the code.

**Mapping mechanism (so this never drifts):** core's `LedgerError` carries the `Code`; the API
holds ONE `map[error]httpStatus` (matched via `errors.Is` against the frozen sentinels). An error
that is not in the map hits the 500 fallback *with a typed body and a logged request-id* — the
fallback is still never bare. A test asserts every exported `core.Err*` sentinel has a row
(fail-closed: a new core error with no mapping is a failing test, not a silent 500).

---

## 4. Router + middleware (stdlib `net/http` + thin router)

### 4.1 Router

Go 1.22+ `http.ServeMux` already does method+pattern routing with path wildcards (`{id}`), so the
"thin router" is the stdlib mux itself — no third-party dependency, honouring Amara's stdlib-only
constraint.

```go
mux := http.NewServeMux()
mux.HandleFunc("POST /accounts",                 h.createAccount)
mux.HandleFunc("GET /accounts/{id}",             h.getBalance)
mux.HandleFunc("POST /transfers",                h.createTransfer)
mux.HandleFunc("GET /accounts/{id}/statement",   h.getStatement)
// unmatched method on a known path → ServeMux returns 405; unknown path → 404.
// Both are run through the JSON-error writer so even these carry the typed body (§3).
```
`id` is read with `r.PathValue("id")`.

### 4.2 Middleware chain

Applied outermost → innermost. Order matters: recover is outermost so it catches panics in
everything below; request-id is next so every log line and error body (including a panic's 500)
carries it; auth is innermost so a rejected request is cheap and never touches a handler.

```
recover → requestID → logging → auth → [handler]
             │            │        │
             │            │        └─ validates bearer, sets principal in ctx, else 401
             │            └─ structured access log, keyed by request-id
             └─ attaches request_id to ctx; echoes X-Request-Id response header
recover ── catches any panic, writes a typed 500 (code "internal"), never leaks a stack to client
```

- **recover**: converts a panic into the typed 500 body (§3), logs stack + request-id server-side.
  This is what guarantees "never a bare 500" even for an unexpected crash.
- **requestID**: generates/propagates `X-Request-Id`; put in `ctx` so handlers and the error
  writer read it.
- **logging**: one structured line per request (method, path, status, principal, duration,
  request-id).
- **auth**: the §2 boundary. Resolves bearer → `PrincipalID`, stores it in `ctx`. No token →
  401 `unauthenticated`, handler never runs. **Health check path (`GET /healthz`) is the one
  enumerated exemption** — an explicit allowlist, never a `path != "/healthz"` skip (CLAUDE.md
  fail-closed rule: enumerate the exempt set).

JSON error writing is not a middleware but a shared helper every handler and the recover/router
paths call, so there is exactly one place that serialises the §3 body.

### 4.3 Handler signature sketches

All handlers share a shape: pull principal from ctx, decode, build the core cmd, call `Ledger`,
then `writeJSON` or `writeErr` (which runs the §3 map). Illustrative, not final:

```go
type Handler struct {
    ledger core.Ledger            // the ONLY domain dependency (Amara §4a)
    admins map[core.PrincipalID]bool
}

func (h *Handler) createTransfer(w http.ResponseWriter, r *http.Request) {
    principal := principalFrom(r.Context())      // set by auth middleware
    key := r.Header.Get("Idempotency-Key")
    if key == "" {
        writeErr(w, r, http.StatusBadRequest, "idempotency_key_required",
            "Idempotency-Key header is required")
        return
    }
    var body transferReq
    if err := decodeJSON(r, &body); err != nil {
        writeErr(w, r, http.StatusBadRequest, "bad_request", err.Error())
        return
    }
    res, err := h.ledger.Transfer(r.Context(), core.TransferCmd{
        Authorizer:     principal,
        From:           core.AccountID(body.From),
        To:             core.AccountID(body.To),
        Amount:         core.Amount(body.Amount),
        Currency:       core.Currency(body.Currency),
        IdempotencyKey: key,
    })
    if err != nil {
        writeCoreErr(w, r, err)                  // runs the §3 errors.Is map
        return
    }
    status := http.StatusCreated                 // 201 first write
    if res.Replayed {
        status = http.StatusOK                   // 200 replay (§5)
    }
    writeJSON(w, status, transferResp(res.Transfer, res.Replayed))
}
```

`writeCoreErr` is the single translation point: `errors.Is(err, core.ErrUnauthorized)` → 403,
and so on down the table, with the unmapped fallback to a typed 500.

---

## 5. Idempotency surface (R4)

### 5.1 How the client passes the key

**`Idempotency-Key: <client-supplied-string>` request header on `POST /transfers`.** A header,
not a body field, because it is a *transport/delivery* property of the request (it governs
retry/replay semantics), it is the same convention Stripe/standard idempotency uses so Nia and
any API client expect it there, and keeping it out of the body means the body is purely the
transfer's financial content. The header value becomes `TransferCmd.IdempotencyKey` verbatim.

**Required.** A transfer with no key is **400 `idempotency_key_required`** — rejected at the API
before core, because an un-keyed transfer cannot be safely retried and R4 is a Hard property of
this system. (Core also requires it; the API fails fast with a clear code.)

### 5.2 What a replay returns

Core keys idempotency on `(owner, key)` and, on a repeat of the *same* key with the *same*
params, returns the original `TransferResult` with `Replayed == true` and **no second posting**
(Amara §3, §4). The API:

- **Replay (`Replayed == true`)** → **200 OK** with the *original* transfer body (same
  `transfer_id`, same postings, same `created_at`). A `"replayed": true` field tells an
  interested client, but the body is otherwise byte-identical to the first response. **This is a
  success, not an error** (Amara §7.3 — confirmed, I do not surface replay as a client error).
- **First write** → **201 Created** with the new transfer.
- **Same key, DIFFERENT params** → core returns `ErrIdempotencyConflict` → **409
  `idempotency_conflict`** (§3). This is the client-error case: the key was reused for a
  materially different transfer, which is a client bug, not a safe retry.

### 5.3 HTTP semantics — no double post

The guarantee a client relies on: **POST the same transfer twice with the same key and you get
the same result once.** Network retry, double-click, at-least-once delivery from Nia's form — all
safe. The API adds nothing of its own on top of core's guarantee; it does not keep its own
idempotency cache (that would be a second source of truth racing core's `UNIQUE(owner, key)`
backstop). The 200-vs-201 split is the only API-level signal, and it is advisory — a client that
treats both as success is correct.

One edge, stated: the key's scope is `(owner, key)` (Amara §4 design notes). Two different
principals may use the same key string without colliding — the API does not need to namespace it,
core already does.

---

## 6. Open questions back to the chain

Ordered by who they block.

1. **[Amara — `Balance` return shape forces a second read] `Ledger.Balance` returns
   `(Amount, error)`, no currency.** My `GET /accounts/{id}` body wants `{amount, currency}`
   (Nia needs the currency to render, Amara §7.6). To get the currency I must either (a) do a
   second lookup — but the `Ledger` port exposes no `GetAccount`; it is on the `Store`/`Tx` ports,
   which the API is not allowed to touch — or (b) have `Balance` return the currency too. **This
   is the one place the frozen contract blocks a clean API.** Options I see, your call since it
   touches §4:
   - **(i)** add `GetAccount(ctx, AccountID) (Account, error)` to the `Ledger` interface — cleanest,
     the API often wants account metadata (owner check display, currency) and today has no port for
     it;
   - **(ii)** change `Balance` to return `(Amount, Currency, error)` or a small `BalanceResult{Amount, Currency}`;
   - **(iii)** I render balance without currency and Nia fetches currency elsewhere — worst, splits
     one logical read into two round-trips.
   I lean **(i)** — it is the smallest contract addition that unblocks every account-read the API
   and UI will want, and it is additive (no existing signature changes). FLAGGING rather than
   diverging. Until you rule, I will assume (i) and sketch the handler against a `GetAccount` seam.

2. **[Amara — confirm 401 vs 403 split] I use 401 for "no/invalid token" (api) and 403 for
   `ErrUnauthorized` (core, ownership).** Your §7.2 mapping only named 403 for `unauthorized`;
   401 is a transport case core never sees. Confirm you are happy that `unauthorized` (the frozen
   code) is strictly the ownership-failure 403, and that the API owns a separate
   `unauthenticated`/401 code core does not emit. No contract change — just confirming the code
   string `unauthorized` is not overloaded.

3. **[Nia — your consumption contract is §1 + §3.]** The endpoint shapes (§1), the error table
   (§3), and the idempotency header (§5) are what you code to. Two asks of you: render `amount` as
   minor-unit int scaled client-side (never parse money as float, Amara §7.6); build statement
   "load more" on `next_after_seq` keyset, never offset. A rejected transfer gives you a stable
   `code` + a `message` — key behaviour off `code`, show `message` (R22).

4. **[Kofi — no direct dependency, one indirect note]** The API never touches your migrations or
   the `Store`. The only thing I rely on that crosses to you: the `SQLITE_BUSY`-after-busy_timeout
   case must surface from the adapter as a *distinguishable* retryable error (Amara §5/§7.5) so my
   §3 map can turn it into **503 `retry_later`** rather than a 500. If that infra error arrives
   indistinguishable from an invariant error, I cannot map it correctly. Flagging so the adapter's
   error type is stable.

5. **[Orchestrator / Amara — admin set for `allow_negative`]** §2.4 gates the privileged flag on a
   configured admin principal set. The *source* of that config (startup flag, env, a seed) is a
   wiring decision in `cmd/ledgerd`. I will default it to empty (no principal may mint via the
   endpoint; the system account is provisioned by seed/config), which is the safe default. Flag if
   you want an explicit admin principal wired instead.

---

*Dakari Mwangi, api/builder. Built to `00-core.md` §4 as frozen. §1 (endpoint→method) and §3
(error→status) are the surfaces Nia codes to. The one place the frozen contract blocks me is §6.1
— raised, not forked.*

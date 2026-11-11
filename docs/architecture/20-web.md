# 20 — Web/UI Architecture (the honest front end)

**Author:** Nia Achieng (`nia-1`, designer) — owner of the `web/` stream, THIRD and LAST link in
the `core → api → web` chain.
**Status:** Design doc. Builds strictly against Dakari's FROZEN HTTP surface (`10-api.md` §1
endpoint shapes, §3 error→status table, §5 idempotency header). I do not invent endpoints; where
the API does not expose something the UI needs, I FLAG it in §5 — I do not fake it with a
client-side guess.
**Consumes:** the JSON request/response shapes in `10-api.md` §1, the stable `code` strings in
`10-api.md` §3, the `Idempotency-Key` header contract in `10-api.md` §5.
**Serves:** a signed-in principal — balances (R20), statement (R21), transfer (R22).
**Stack:** React 18 + TypeScript (strict), functional components only, Vite dev server, **bun**
as package manager and runner. No component library mandated; plain accessible markup.

The one belief this whole doc is organised around: **a money UI that lies about why something
failed is worse than no UI.** A rejected transfer is a *typed event* with a stable `code` — the
front end's job is to turn that code into the one true sentence for the human, placed where they
can act on it. "Something went wrong" is a bug here, not a fallback.

---

## 1. Stack, structure, and how it talks to the API

### 1.1 Component tree

```
<App>                         # provides AuthContext + ApiClient, top-level routing
├── <SignIn>                  # token entry (toy: paste bearer token); sets principal
└── <AuthedShell>             # only mounts when a token is present
    ├── <AccountsPane>        # R20 — list of my accounts + balances
    │   └── <AccountRow>      #        one account: id, balance (scaled), currency, select
    ├── <StatementPane>       # R21 — postings for the selected account, keyset "load more"
    │   ├── <PostingRow>
    │   └── <LoadMoreButton>
    └── <TransferForm>        # R22 — from/to/amount/currency, inline typed errors
        └── <FieldError> / <FormError>   # where typed codes render (§3)
```

`AccountsPane` owns the selected-account id and lifts it to `AuthedShell` so `StatementPane` and
`TransferForm` can read it (select an account → its statement loads; its id pre-fills the transfer
`from`). State is local React state + a tiny context for auth and the client; **no Redux, no React
Query mandated for a toy this size** — a `useApi` hook with `useState`/`useEffect` is enough, and
keeps the data flow legible for review.

### 1.2 The typed API client (one place that knows the wire)

Every network call goes through one typed client. Nothing else in the UI constructs a URL, sets a
header, or parses an error body — that is how the §3 error contract stays enforced in exactly one
place (the mirror of Dakari's "one `errors.Is` map" discipline on the server).

```ts
// web/src/api/types.ts — mirrors 10-api.md §1 response bodies verbatim.
export type Minor = number;            // int64 minor units. NEVER a display float. See §4.

export interface Account {
  id: string; owner: string; currency: string;
  allow_negative: boolean; created_at: string;
}
export interface BalanceBody   { account_id: string; amount: Minor; currency: string; }
export interface Posting       { id: string; transfer_id: string; amount: Minor;
                                 currency: string; seq: number; created_at: string; }
export interface StatementBody { account_id: string; postings: Posting[];
                                 next_after_seq: number; has_more: boolean; }
export interface TransferBody  { transfer_id: string; from: string; to: string;
                                 amount: Minor; currency: string; replayed: boolean;
                                 created_at: string; postings: Posting[]; }

// The one error shape, every endpoint (10-api.md §3).
export interface ApiErrorBody { error: { code: string; message: string; request_id: string }; }

// A typed throwable the UI branches on by `code`, never by HTTP status.
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,        // stable string from §3 — the thing the UI keys off
    readonly userMessage: string, // server `message`, shown to the human
    readonly requestId: string,   // quote in a support/debug affordance
  ) { super(`${code} (${status})`); }
}
```

```ts
// web/src/api/client.ts — the only module that touches fetch().
export interface ApiClient {
  getAccount(id: string): Promise<Account>;                        // GET /accounts/{id} (metadata)
  getBalance(id: string): Promise<BalanceBody>;                    // GET /accounts/{id}  -> {amount,currency}
  getStatement(id: string, page: Page): Promise<StatementBody>;    // GET /accounts/{id}/statement
  createTransfer(req: TransferReq, idemKey: string): Promise<TransferBody>; // POST /transfers
  createAccount(req: CreateAccountReq): Promise<Account>;          // POST /accounts (not a primary view)
}
export interface Page { limit: number; afterSeq: number; }
export interface TransferReq { from: string; to: string; amount: Minor; currency: string; }

export function makeClient(baseUrl: string, getToken: () => string | null): ApiClient {
  async function call<T>(method: string, path: string, opts: {
    body?: unknown; idemKey?: string;
  } = {}): Promise<T> {
    const token = getToken();
    const res = await fetch(baseUrl + path, {
      method,
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),   // 10-api.md §2
        ...(opts.body ? { "Content-Type": "application/json" } : {}),
        ...(opts.idemKey ? { "Idempotency-Key": opts.idemKey } : {}), // 10-api.md §5
      },
      body: opts.body ? JSON.stringify(opts.body) : undefined,
    });
    if (!res.ok) {
      // Parse the ONE error shape. If the body is not even JSON, still surface a typed error
      // rather than leak a raw status — but never pretend we know the code.
      const e = (await res.json().catch(() => null)) as ApiErrorBody | null;
      throw new ApiError(
        res.status,
        e?.error.code ?? "unknown",
        e?.error.message ?? "The server returned an unreadable error.",
        e?.error.request_id ?? "",
      );
    }
    return res.json() as Promise<T>;
  }
  return {
    getAccount:   (id)        => call("GET",  `/accounts/${id}`),
    getBalance:   (id)        => call("GET",  `/accounts/${id}`),      // §4a: GetAccount+Balance -> {amount,currency}
    getStatement: (id, p)     => call("GET",  `/accounts/${id}/statement?limit=${p.limit}&after_seq=${p.afterSeq}`),
    createTransfer: (r, k)    => call("POST", `/transfers`, { body: r, idemKey: k }),
    createAccount:  (r)       => call("POST", `/accounts`,  { body: r }),
  };
}
```

> Note on `getAccount` vs `getBalance`: Dakari's `GET /accounts/{id}` now returns the combined
> `{account_id, amount, currency}` body (he resolved his own open question 6.1 — `GetAccount` was
> added to the frozen `Ledger` so the balance body carries currency). The UI needs exactly that one
> read for R20. I keep a nominal `getAccount` typed against the richer `Account` shape only for the
> account-metadata case; the balances view uses `getBalance`. See §5 open question (1) — I still
> need a *list* of my accounts, which no endpoint yet returns.

### 1.3 Auth token handling

The toy auth is a bearer token whose subject is the principal (`10-api.md` §2.1). The front end:

- Holds the token in `AuthContext`, backed by `sessionStorage` (not `localStorage`) so it clears
  on tab close and never outlives the session — a money toy should not leave a bearer token
  sitting in persistent storage.
- Attaches it as `Authorization: Bearer <token>` on every call (done once, in the client).
- On a **401 `unauthenticated`** response, clears the token and routes to `<SignIn>` — the token is
  no longer valid, re-auth is the only cure (§3).
- Never logs the token, never puts it in a URL query param.

Toy scope, stated so a reviewer does not flag a missing feature as a defect: no refresh, no expiry
countdown, no OAuth redirect — the sign-in is "paste the token the roster seeded for your
principal". This mirrors Dakari's explicit toy boundary (`10-api.md` §2.1).

---

## 2. The three views

### 2.1 Balances view (R20) — `GET /accounts/{id}` → `{amount, currency}`

A signed-in principal sees each of their accounts with its derived balance. One row per account;
the balance is rendered by scaling the int64 minor-unit `amount` client-side (§4) — the display
never does float math on money.

```tsx
function AccountRow({ account, onSelect }: { account: BalanceBody; onSelect: (id: string) => void }) {
  return (
    <button className="account-row" onClick={() => onSelect(account.account_id)}>
      <span className="acct-id">{account.account_id}</span>
      <span className="acct-balance">{formatMinor(account.amount, account.currency)}</span>
      <span className="acct-ccy">{account.currency}</span>
    </button>
  );
}
```

A zero balance renders as `0.00`, a system/mint account's negative balance renders with a leading
`-` and is visually flagged (it is legitimately negative — `allow_negative`), never hidden. An
account that 404s (`account_not_found`) is dropped from the list with a quiet inline note, not an
error banner — a stale id in my list is not a failure of the whole view.

**Gap:** there is no "list my accounts" endpoint; I reach a single account by id. See §5 (1).

### 2.2 Statement view (R21) — `GET /accounts/{id}/statement`, keyset "load more"

Selecting an account loads its postings in `seq` order, paged by keyset per Dakari's §1.4 /
Amara's §6. **No offset anywhere** — "load more" passes the previous response's `next_after_seq`
back as `after_seq`, exactly as the frozen contract requires.

```tsx
function StatementPane({ accountId, api }: { accountId: string; api: ApiClient }) {
  const [rows, setRows] = useState<Posting[]>([]);
  const [afterSeq, setAfterSeq] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [err, setErr] = useState<ApiError | null>(null);

  async function loadMore() {
    try {
      const page = await api.getStatement(accountId, { limit: 50, afterSeq });
      setRows(prev => [...prev, ...page.postings]);
      setAfterSeq(page.next_after_seq);   // keyset cursor, never a page number
      setHasMore(page.has_more);
    } catch (e) { setErr(e as ApiError); } // e.g. 403 not-your-account, 404 unknown id
  }
  // reset + first load whenever accountId changes…
}
```

Each `PostingRow` shows the signed amount (debits negative/red, credits positive), the
`transfer_id` (links the two legs of a transfer in the mind of the reader), `seq`, and
`created_at`. A statement for an account the principal does not own returns **403 `unauthorized`**
— rendered as a "this is not your account" state, distinct from an empty statement (§3).

### 2.3 Transfer form (R22) — `POST /transfers` with a generated `Idempotency-Key`

The form collects `from`, `to`, `amount` (entered in major units, parsed to minor — §4),
`currency`. It generates the idempotency key and attaches it as the header (`10-api.md` §5).

```tsx
function TransferForm({ api, defaultFrom }: { api: ApiClient; defaultFrom?: string }) {
  const [form, setForm] = useState({ from: defaultFrom ?? "", to: "", amount: "", currency: "USD" });
  const [idemKey, setIdemKey] = useState(() => crypto.randomUUID()); // one key per transfer intent
  const [state, setState] = useState<"idle" | "sending" | "done">("idle");
  const [fieldErr, setFieldErr] = useState<Partial<Record<"from" | "to" | "amount" | "form", string>>>({});

  async function submit() {
    const amount = parseMinor(form.amount, form.currency);  // §4 — rejects float drift locally
    if (amount == null) { setFieldErr({ amount: "Enter a valid amount, e.g. 25.00" }); return; }
    setState("sending");
    try {
      const res = await api.createTransfer(
        { from: form.from, to: form.to, amount, currency: form.currency }, idemKey);
      setState("done");           // res.replayed === true is still success — show "already done"
      // DO NOT rotate idemKey here: a retry of THIS same intent must reuse the key (R4).
    } catch (e) {
      setFieldErr(mapTransferError(e as ApiError));  // §3 — typed, placed, never generic
      setState("idle");
    }
  }
}
```

**Idempotency-Key discipline (the subtle, load-bearing part):**

- One key is generated **per transfer intent**, when the form is readied. A network retry, a
  double-click, or a "the response never came, try again" all reuse the **same** key — that is the
  whole point of R4, and it is why the key is `useState`, generated once, not regenerated on each
  submit.
- A **200 replay** (`res.replayed === true`) is rendered as success with a quiet "this transfer
  was already completed" note — never an error. This matches Dakari §5.2 exactly: replay is a 2xx.
- The key is rotated to a fresh `crypto.randomUUID()` **only** when the user explicitly starts a
  *new* transfer (resets the form), never on a retry of the current one.
- `409 idempotency_conflict` (same key, different params) is the one idempotency *error* — it
  means a client bug reused a key for a materially different transfer. Rendered as a form-level
  error telling the user to start the transfer again (which rotates the key). See §3.

---

## 3. Honest error states — the code → UI-message contract (R22, R13)

This is the heart of the stream. The UI branches on the stable `code` string from `10-api.md`
§3, **never on HTTP status** (the error body shape is identical across statuses, so status is only
used to decide *re-auth vs inline*, below). Every row maps to one specific human sentence and one
specific placement.

### 3.1 The table

| `code` (from API §3) | HTTP | Where it shows | UI message to the human |
|---|---|---|---|
| `insufficient_funds` | 409 | inline, under the **amount** field | "Not enough funds in the source account. Available balance is less than this transfer." |
| `currency_mismatch` | 422 | inline, under the **currency** field | "These accounts use different currencies. A transfer must be in one currency." |
| `unbalanced` | 422 | form-level | "This transfer did not balance. Please re-enter the amount and try again." (should be rare from a single-amount form — see note) |
| `unauthorized` | 403 | inline, under the **from** field | "You do not own the source account, so you cannot transfer from it." |
| `account_not_found` | 404 | inline, under **from** or **to** (whichever is unknown) | "That account does not exist. Check the account id." |
| `idempotency_conflict` | 409 | form-level | "This looks like a different transfer reusing an earlier reference. Start the transfer again." (rotates the key) |
| `idempotency_key_required` | 400 | form-level (self-bug) | "Internal error: the transfer reference was missing. Please retry." (should never reach the user — the client always sends it) |
| `bad_request` | 400 | form-level | "The request was malformed. Please retry; if it persists, contact support with id {request_id}." |
| `unprocessable_entity` | 422 | inline, highlight the empty required field | "A required field is missing." |
| `forbidden_privileged_flag` | 403 | form-level | "You are not allowed to set that option." (not reachable from this form; defensive) |
| `retry_later` | 503 | form-level, with a **Retry** button | "The system is busy. Your transfer was not made. Try again in a moment." (safe to retry — same key) |
| `unauthenticated` | 401 | **route to Sign-In**, not inline | "Your session has expired. Please sign in again." |
| `internal` / `unknown` | 500 / — | form-level, last resort | "Something unexpected happened on our side (id {request_id}). Your transfer was not made." |

Only `internal`/`unknown` gets the generic sentence — and even it quotes the `request_id` and
asserts the financial fact that matters ("your transfer was not made"). Every business rejection
gets its real reason. That is the R22 bar.

### 3.2 The three placement classes (401 vs 403 vs 409/422)

Dakari's status split maps cleanly onto three UI behaviours — this is exactly why he kept 401 and
403 distinct (`10-api.md` §2.3):

- **401 `unauthenticated` → re-auth.** The identity itself is unestablished. Clear the token, route
  to `<SignIn>`. This is a session-level event, not a form error — showing it inline would be a
  lie (the field is fine; the session is gone).
- **403 (`unauthorized`, `forbidden_privileged_flag`) → inline, not re-auth.** We know who you are;
  you just cannot do this. Re-authenticating would not help, so we do NOT bounce to sign-in. The
  `unauthorized` case pins to the `from` field because that is the account the ownership check
  failed on.
- **409 / 422 (`insufficient_funds`, `currency_mismatch`, `unbalanced`, `idempotency_conflict`) →
  inline business rejection on the form.** The request was understood and refused for a stated
  reason. The form stays filled, the typed reason appears at the relevant field, the user can
  correct and resubmit. These never navigate away.

```ts
// web/src/views/transferErrors.ts — the ONLY place codes become UI copy/placement.
type Placement = { field?: "from" | "to" | "amount" | "currency"; form?: boolean; reauth?: boolean };

const TRANSFER_ERRORS: Record<string, { msg: string; at: Placement }> = {
  insufficient_funds:   { at: { field: "amount" },   msg: "Not enough funds in the source account…" },
  currency_mismatch:    { at: { field: "currency" }, msg: "These accounts use different currencies…" },
  unauthorized:         { at: { field: "from" },     msg: "You do not own the source account…" },
  account_not_found:    { at: { field: "from" },     msg: "That account does not exist…" },
  unbalanced:           { at: { form: true },         msg: "This transfer did not balance…" },
  idempotency_conflict: { at: { form: true },         msg: "This looks like a different transfer…" },
  retry_later:          { at: { form: true },         msg: "The system is busy. Try again…" },
  unauthenticated:      { at: { reauth: true },       msg: "Your session has expired…" },
};

export function mapTransferError(e: ApiError) {
  const entry = TRANSFER_ERRORS[e.code];
  if (!entry) return { form: `Unexpected error (id ${e.requestId}). Your transfer was not made.` };
  if (entry.at.reauth) { /* AuthContext.signOut(); route to SignIn */ return {}; }
  return entry.at.field ? { [entry.at.field]: entry.msg } : { form: entry.msg };
}
```

A single `test_transfer_error_map` asserts **every** `code` Dakari lists in §3 that can reach a
transfer has a row here — fail-closed, the same discipline as his server-side "every sentinel has a
status row" test. A new code with no UI mapping is a failing test, not a silent generic banner.

> On `unbalanced` from this form: the single-amount form always sends a balanced two-leg transfer,
> so `unbalanced` should be unreachable here. I still map it rather than let it fall to the generic
> bucket — if the API shape ever grows multi-leg entries, the honest message is already wired.

---

## 4. Money and precision — int64 minor units, end to end, no float drift

The rule from both upstream docs (Amara §2, Dakari §1): **money is `int64` minor units on the
wire; never a float, never a decimal string.** The front end honours this on both sides of the I/O:

- **Over the wire:** `amount` is a JSON integer. TypeScript types it as `Minor = number`. JS numbers
  are IEEE-754 doubles, which represent every integer exactly up to 2^53 — far beyond any toy
  balance — so a minor-unit integer round-trips losslessly. I do **not** reach for BigInt for the
  toy, but I state the boundary: if a balance could exceed 2^53 minor units, `Minor` becomes
  `bigint` and the client parses with a custom reviver. Flagged, not needed now.
- **Display math is integer-only.** Formatting never divides a float. I split into whole and
  fractional parts by integer arithmetic against the currency's scale:

```ts
// web/src/money.ts — scale is minor-units-per-major. Toy assumes 2 for all currencies;
// a real system reads it per-currency. No floating-point division of the amount, ever.
const SCALE: Record<string, number> = { USD: 2, GBP: 2, EUR: 2, JPY: 0 };

export function formatMinor(amount: Minor, currency: string): string {
  const digits = SCALE[currency] ?? 2;
  const unit = 10 ** digits;                 // integer
  const neg = amount < 0;
  const abs = Math.abs(amount);
  const whole = Math.trunc(abs / unit);      // integer division
  const frac = abs - whole * unit;           // integer remainder — never a float %
  const fracStr = digits ? "." + String(frac).padStart(digits, "0") : "";
  return `${neg ? "-" : ""}${whole.toLocaleString()}${fracStr}`;
}

// Parse user input ("25.00", "25", "25.5") -> integer minor units, or null if it does not
// represent an exact minor-unit amount. We parse the STRING, we never Number()-then-multiply.
export function parseMinor(input: string, currency: string): Minor | null {
  const digits = SCALE[currency] ?? 2;
  const m = input.trim().match(/^(-?)(\d+)(?:\.(\d+))?$/);
  if (!m) return null;
  const [, sign, whole, frac = ""] = m;
  if (frac.length > digits) return null;                 // more precision than the currency allows
  const scaled = whole + frac.padEnd(digits, "0");       // string concat, not float multiply
  const value = Number(scaled);
  if (!Number.isSafeInteger(value)) return null;
  return sign === "-" ? -value : value;
}
```

The key move in `parseMinor`: the amount is assembled by **string manipulation** (pad the
fractional digits, concatenate, parse once as an integer), so `"25.10"` becomes `2510` with no
`25.1 * 100 = 2510.0000000001` hazard. Over-precise input (`"25.001"` for a 2-digit currency) is
rejected at the field rather than silently truncated — honest about what it cannot represent.
`money.ts` is unit-tested with a table of edge cases (zero, negative, `JPY` scale 0, trailing
zeros, over-precision, max safe integer).

---

## 5. Open questions back up the chain

Ordered by who they block.

1. **[Dakari — no "list my accounts" endpoint] The balances view (R20) is "a principal sees *their
   accounts*", plural, but the frozen API only exposes `GET /accounts/{id}` — fetch-by-id.** There
   is no `GET /accounts` (list for the authenticated principal). Today the UI can only render
   accounts whose ids it already knows, which means something outside the API has to hand the user
   their id list — not a real "my accounts" view. Options I see, your call since it is a new
   endpoint on your surface:
   - **(i)** add `GET /accounts` returning the authenticated principal's accounts (owner filter is
     the principal from the bearer token, so no new auth surface) — cleanest, directly satisfies
     R20's plural, and is a natural read Dakari's handler can serve off a `Store` list method;
   - **(ii)** I keep a client-side list of ids the user has created/been given this session — fails
     the moment they reload or use a second device, and makes R20 depend on UI memory rather than
     the ledger. Not honest.
   I lean **(i)**. Until you rule, I will build `AccountsPane` against a `getMyAccounts()` client
   seam that today is fed by a provided id list, and swap it to `GET /accounts` the moment it
   exists — the component does not change, only the client method. FLAGGING, not inventing the
   endpoint in the client.

2. **[Dakari — is `GET /accounts/{id}` on an account I do NOT own a 403 or a 404?]** §3 lists
   `account_not_found` (404) and `unauthorized` (403), but for a *read* of someone else's account
   the table does not say which fires. It matters for copy: 403 → "not your account", 404 → "no
   such account". Balance and statement reads need the same answer. If reads are not ownership-gated
   at all (any authenticated principal may read any balance), tell me — then there is no 403 on a
   read and I drop that state. Confirm the read-side ownership behaviour.

3. **[Dakari — a human-facing `from`/`to` identifier?]** The transfer form asks the user to type an
   opaque `acc_01H...` id. That is fine for the toy, but if there is any notion of an account label
   or the owner's principal name being resolvable, the form could show it for confirmation ("you
   are sending to alice's USD account") before submit — a real money-UI guardrail against sending
   to the wrong id. No endpoint exposes this today and I am **not** inventing one; flagging it as a
   UX gap the API could later close with account metadata on the list endpoint from (1).

4. **[Amara/Dakari — per-currency minor-unit scale]** §4 hardcodes a `SCALE` map (USD=2, JPY=0).
   The ledger treats `currency` as an opaque label (Amara §2) and never tells the UI its scale, so
   the front end is guessing the number of minor digits. For the toy's USD-centric demo this is
   fine, but it is a UI-side assumption about a core concept. Flagging: if currencies beyond the
   common 2-digit ones matter, the scale should originate somewhere authoritative, not in a
   front-end constant.

---

*Nia Achieng, web/designer. Built to `10-api.md` §1 (shapes), §3 (error table) and §5 (idempotency
header) as frozen. The three views in §2 and the code→message contract in §3 are what the UI
guarantees the human. The one thing the frozen API does not yet give me is a way to LIST a
principal's accounts (§5.1) — raised, not faked.*

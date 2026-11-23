// harness/reverse-stub.mjs — a contract stub for dakari's POST /entries/{id}/reverse.
//
// WHY THIS EXISTS. The reversal endpoint is being built in a parallel stream and is not
// on dev. Reviewing this UI from the code instead would break the repo's UI verification
// contract ("a changed surface left unrendered fails the review"), so the harness brings
// the surface up for real: everything proxies to a live ledgerd on real SQLite data, and
// only the one unbuilt endpoint is served here.
//
// It is a stub of the CONTRACT, not a second ledger. The compensating transfer it creates
// is a real POST /transfers against ledgerd, authorised by the caller's own token, so the
// money that moves is the ledger's and the 403 is the ledger's answer, not a fake one.
//
// DELETE THIS DIRECTORY once POST /entries/{id}/reverse lands: point VITE_API_TARGET back
// at ledgerd and nothing else changes.

const LEDGERD = process.env.LEDGERD_URL ?? "http://localhost:8080";
const PORT = Number(process.env.STUB_PORT ?? 8081);

// What the stub has learned by watching traffic go past. None of it is authoritative;
// it exists only so the stub can answer the three questions the contract poses.
const entries = new Map(); // posting id -> { account_id, transfer_id, amount, currency }
const transfers = new Map(); // transfer id -> { from, to, amount, currency }
const reversedBy = new Map(); // reversed posting id -> compensating transfer id
const reverses = new Map(); // compensating transfer id -> reversed posting id

function errBody(code, message) {
  return JSON.stringify({ error: { code, message, request_id: `stub_${Date.now()}` } });
}

function jsonRes(status, body) {
  return new Response(typeof body === "string" ? body : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function bearer(req) {
  const h = req.headers.get("authorization") ?? "";
  return h.startsWith("Bearer ") ? h.slice(7) : null;
}

// remember records what a proxied response taught us.
function rememberStatement(body) {
  for (const p of body.postings ?? []) {
    if (p.id) entries.set(p.id, p);
  }
}

function rememberTransfer(body) {
  if (!body.transfer_id) return;
  transfers.set(body.transfer_id, {
    from: body.from,
    to: body.to,
    amount: body.amount,
    currency: body.currency,
  });
}

// annotate injects the reversal link fields the real statement projection will carry.
function annotateStatement(body) {
  for (const p of body.postings ?? []) {
    if (!p.id) continue;
    const compTransfer = reversedBy.get(p.id);
    if (compTransfer !== undefined) p.reversed_by_entry_id = compensatingEntryFor(compTransfer, p.account_id) ?? p.id;
    const reversedEntry = p.transfer_id !== undefined ? reverses.get(p.transfer_id) : undefined;
    if (reversedEntry !== undefined) p.reverses_entry_id = reversedEntry;
  }
  return body;
}

function compensatingEntryFor(transferId, accountId) {
  for (const [id, p] of entries) {
    if (p.transfer_id === transferId && p.account_id === accountId) return id;
  }
  return null;
}

async function proxy(req, url) {
  const target = LEDGERD + url.pathname + url.search;
  const init = { method: req.method, headers: new Headers(req.headers) };
  init.headers.delete("host");
  if (req.method !== "GET" && req.method !== "HEAD") init.body = await req.arrayBuffer();
  const res = await fetch(target, init);

  const ct = res.headers.get("content-type") ?? "";
  if (!ct.includes("application/json")) return res;

  const body = await res.json();
  if (res.ok && /\/statement$/.test(url.pathname)) {
    rememberStatement(body);
    annotateStatement(body);
  }
  if (res.ok && url.pathname === "/transfers") rememberTransfer(body);
  return jsonRes(res.status, body);
}

// --- the one stubbed endpoint -------------------------------------------------
// Rewritten against the LANDED core.Ledger.Reverse (core/reversal.go), not against the
// shape this stream guessed. zola's round-1 C3 was that the stub was self-confirming: it
// returned the same codes the UI expected because both came from the same guess. These
// rules are now read off core, so a UI mismatch would actually show up here:
//
//   core/reversal.go:18-24  the compensating entry's id is DERIVED: rev_<entryID>, and
//                           that is what makes "already reversed" a lookup, not a key check
//   core/reversal.go:64-68  already-reversed -> ErrAlreadyReversed
//   core/reversal.go:72-78  the authorizer must own original.To — where the money WENT —
//                           NOT the account the row sits on. This stub had that wrong.
//   core/reversal.go:21-23  reversing a reversal IS allowed; it is a distinct entry
//
// It is still a stub: dakari's POST /entries/{id}/reverse does not exist, so the mandatory
// live reversal e2e remains unrun (C3). Delete this file when the route lands.
const REVERSAL_PREFIX = "rev_"; // core/reversal.go reversalPrefix
const reversalIdFor = (transferId) => REVERSAL_PREFIX + transferId;

async function reverse(req, entryId) {
  const token = bearer(req);
  if (token === null) return jsonRes(401, errBody("unauthenticated", "missing bearer token"));

  const entry = entries.get(entryId);
  if (entry === undefined) {
    return jsonRes(404, errBody("entry_not_found", "no such entry on this account"));
  }

  const original = transfers.get(entry.transfer_id);
  if (original === undefined) {
    return jsonRes(422, errBody("unprocessable_entity", "the original transfer is not known to the stub"));
  }

  // Already reversed, keyed on the DERIVED id the way core does it.
  if (transfers.has(reversalIdFor(entry.transfer_id)) || reversedBy.has(entryId)) {
    return jsonRes(409, errBody("already_reversed", "this entry has already been reversed"));
  }

  // R7 binds on the DEBITED account, which is the original's destination. Asked of the
  // ledger rather than decided here: a non-owned read is 404 (D-D1), and the reverse
  // ACTION turns that into 403, because we are telling the caller they may not do this.
  const own = await fetch(`${LEDGERD}/accounts/${encodeURIComponent(original.to)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (own.status === 404 || own.status === 403) {
    return jsonRes(403, errBody("unauthorized", "you do not own the account this reversal would debit"));
  }

  // The compensating transfer: the same money, the other way, authorised by the caller.
  const res = await fetch(`${LEDGERD}/transfers`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
      "Idempotency-Key": req.headers.get("idempotency-key") ?? `stub-rev-${entryId}`,
    },
    body: JSON.stringify({
      from: original.to,
      to: original.from,
      amount: original.amount,
      currency: original.currency,
    }),
  });
  const body = await res.json();
  if (!res.ok) return jsonRes(res.status, body); // the ledger's own typed rejection, passed through

  rememberTransfer(body);
  reversedBy.set(entryId, body.transfer_id);
  reverses.set(body.transfer_id, entryId);

  return jsonRes(201, {
    transfer_id: body.transfer_id,
    reverses_entry_id: entryId,
    amount: body.amount,
    currency: body.currency,
    replayed: body.replayed ?? false,
    created_at: body.created_at,
    postings: body.postings ?? [],
  });
}

Bun.serve({
  port: PORT,
  async fetch(req) {
    const url = new URL(req.url);
    const m = url.pathname.match(/^\/entries\/([^/]+)\/reverse$/);
    if (m !== null && req.method === "POST") return reverse(req, decodeURIComponent(m[1]));
    return proxy(req, url);
  },
});

console.log(`reverse-stub: proxying ${LEDGERD}, stubbing POST /entries/{id}/reverse, on :${PORT}`);

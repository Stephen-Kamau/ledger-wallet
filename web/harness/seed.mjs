// harness/seed.mjs — puts real data in a real ledger so the statement has rows to reverse.
// Alice owns two accounts (so she can authorise a reversal, which debits the account the
// money went TO); Bob owns one, which is what makes the 403 not-your-account path real
// rather than simulated.

const API = process.env.LEDGERD_URL ?? "http://localhost:8080";
// Idempotency keys are scoped to this seed run. Fixed keys would make a second run
// against the same database a 409 idempotency_conflict (same key, different accounts),
// which is correct ledger behaviour and a useless harness.
const RUN = process.env.SEED_RUN ?? Date.now().toString(36);

async function call(token, method, path, body, idem) {
  const headers = { Authorization: `Bearer ${token}` };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (idem !== undefined) headers["Idempotency-Key"] = idem;
  const res = await fetch(API + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  if (!res.ok) throw new Error(`${method} ${path} -> ${res.status} ${text}`);
  return JSON.parse(text);
}

const alice1 = await call("alice", "POST", "/accounts", { currency: "USD" });
const alice2 = await call("alice", "POST", "/accounts", { currency: "USD" });
const bob1 = await call("bob", "POST", "/accounts", { currency: "USD" });
// The mint account may go negative; it is what puts money into the toy in the first place.
const mint = await call("mint", "POST", "/accounts", { currency: "USD", allow_negative: true });

await call("mint", "POST", "/transfers", { from: mint.id, to: alice1.id, amount: 250000, currency: "USD" }, `seed-${RUN}-mint-1`);
await call("mint", "POST", "/transfers", { from: mint.id, to: bob1.id, amount: 90000, currency: "USD" }, `seed-${RUN}-mint-2`);

// Three transfers between Alice's own accounts: reversible, because she authorises both legs.
await call("alice", "POST", "/transfers", { from: alice1.id, to: alice2.id, amount: 42500, currency: "USD" }, `seed-${RUN}-a-1`);
await call("alice", "POST", "/transfers", { from: alice1.id, to: alice2.id, amount: 1099, currency: "USD" }, `seed-${RUN}-a-2`);
await call("alice", "POST", "/transfers", { from: alice2.id, to: alice1.id, amount: 7500, currency: "USD" }, `seed-${RUN}-a-3`);
// One to Bob: Alice can see it on her statement but cannot reverse it, because the money
// would have to come back out of Bob's account. This is the 403 row.
await call("alice", "POST", "/transfers", { from: alice1.id, to: bob1.id, amount: 15000, currency: "USD" }, `seed-${RUN}-a-4`);

console.log(JSON.stringify({ alice1: alice1.id, alice2: alice2.id, bob1: bob1.id, mint: mint.id }, null, 2));

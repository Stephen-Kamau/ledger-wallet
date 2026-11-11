// views/TransferForm.tsx — R22 transfer form. POST /transfers with a generated
// Idempotency-Key (api/handlers.go requires the header; R4). A rejection shows the TYPED
// reason by `code` (transferErrors.ts), placed at the field it concerns — never a generic
// failure. 401 routes to re-auth; 403/409/422 are inline business rejections (§3.2).
//
// Idempotency-Key discipline (the load-bearing part):
//  - ONE key per transfer INTENT, held in state. A retry / double-click / "did it go
//    through?" resubmit reuses the SAME key — that is the whole point of R4.
//  - A 200 replay (res.replayed === true) is SUCCESS, shown as "already completed".
//  - The key rotates to a fresh crypto.randomUUID() ONLY when the user starts a NEW
//    transfer (resets the form), never on a retry of the current one.

import { useEffect, useState } from "react";
import type { ApiClient } from "../api/client";
import { newIdempotencyKey } from "../api/client";
import { ApiError, type TransferBody } from "../api/types";
import { formatMinor, parseMinor } from "../money";
import { mapTransferError, type MappedTransferError } from "./transferErrors";

interface FormState {
  from: string;
  to: string;
  amount: string; // major-unit text; parsed to int64 minor on submit (money.parseMinor)
  currency: string;
}

type Phase = "idle" | "sending" | "done";

export function TransferForm({
  client,
  defaultFrom,
  onReauth,
}: {
  client: ApiClient;
  defaultFrom: string;
  onReauth: () => void;
}) {
  const [form, setForm] = useState<FormState>({ from: defaultFrom, to: "", amount: "", currency: "USD" });
  const [idemKey, setIdemKey] = useState<string>(() => newIdempotencyKey()); // one key per intent
  const [phase, setPhase] = useState<Phase>("idle");
  const [errs, setErrs] = useState<MappedTransferError>({});
  const [result, setResult] = useState<TransferBody | null>(null);

  // Keep `from` in sync with the selected account until the user edits it themselves.
  useEffect(() => {
    setForm((f) => (f.from === "" ? { ...f, from: defaultFrom } : f));
  }, [defaultFrom]);

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  // startNew resets the form AND rotates the idempotency key — this is the ONLY place the
  // key rotates (a new intent gets a new key; a retry of the current one does not).
  function startNew() {
    setForm({ from: defaultFrom, to: "", amount: "", currency: "USD" });
    setIdemKey(newIdempotencyKey());
    setErrs({});
    setResult(null);
    setPhase("idle");
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErrs({});

    const amount = parseMinor(form.amount, form.currency); // §4 — rejects float drift locally
    if (amount === null) {
      setErrs({ amount: `Enter a valid amount for ${form.currency}, e.g. 25.00` });
      return;
    }
    if (amount <= 0) {
      setErrs({ amount: "Enter an amount greater than zero." });
      return;
    }
    if (form.from.trim() === "") {
      setErrs({ from: "Enter the source account id." });
      return;
    }
    if (form.to.trim() === "") {
      setErrs({ to: "Enter the destination account id." });
      return;
    }

    setPhase("sending");
    try {
      const res = await client.createTransfer(
        { from: form.from.trim(), to: form.to.trim(), amount, currency: form.currency },
        idemKey, // reuse the SAME key on every retry of this intent (R4)
      );
      setResult(res);
      setPhase("done");
    } catch (e2) {
      const mapped = mapTransferError(e2 as ApiError);
      if (mapped.reauth) {
        onReauth();
        return;
      }
      setErrs(mapped);
      setPhase("idle"); // stay on the form, filled, so the user can correct and resubmit
    }
  }

  if (phase === "done" && result !== null) {
    return (
      <div className="pane">
        <h2>Transfer</h2>
        <div className={`transfer-done${result.replayed ? " replayed" : ""}`}>
          {result.replayed ? (
            <p>This transfer was already completed — no second posting was made.</p>
          ) : (
            <p>Transfer complete.</p>
          )}
          <dl>
            <div>
              <dt>Amount</dt>
              <dd>{formatMinor(result.amount, result.currency)} {result.currency}</dd>
            </div>
            <div>
              <dt>From</dt>
              <dd className="mono">{result.from}</dd>
            </div>
            <div>
              <dt>To</dt>
              <dd className="mono">{result.to}</dd>
            </div>
            <div>
              <dt>Reference</dt>
              <dd className="mono truncate">{result.transfer_id}</dd>
            </div>
          </dl>
          <button onClick={startNew}>New transfer</button>
        </div>
      </div>
    );
  }

  const sending = phase === "sending";
  return (
    <div className="pane">
      <h2>Transfer</h2>
      <form onSubmit={submit} noValidate>
        <Field label="From" error={errs.from}>
          <input value={form.from} onChange={(e) => set("from", e.target.value)} placeholder="source account id" />
        </Field>
        <Field label="To" error={errs.to}>
          <input value={form.to} onChange={(e) => set("to", e.target.value)} placeholder="destination account id" />
        </Field>
        <Field label="Amount" error={errs.amount}>
          <input
            value={form.amount}
            onChange={(e) => set("amount", e.target.value)}
            inputMode="decimal"
            placeholder="25.00"
          />
        </Field>
        <Field label="Currency" error={errs.currency}>
          <input value={form.currency} onChange={(e) => set("currency", e.target.value.toUpperCase())} />
        </Field>

        {errs.form !== undefined && <p className="error-banner">{errs.form}</p>}

        <button type="submit" disabled={sending}>
          {sending ? "Sending…" : "Send transfer"}
        </button>
      </form>
    </div>
  );
}

function Field({
  label,
  error,
  children,
}: {
  label: string;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={`field${error !== undefined ? " has-error" : ""}`}>
      <label>{label}</label>
      {children}
      {error !== undefined && <span className="field-error">{error}</span>}
    </div>
  );
}

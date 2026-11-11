// views/AccountsPane.tsx — R20 balances view. GET /accounts (D-C1) lists the signed-in
// principal's accounts, then GET /accounts/{id} derives each balance. int64 minor units
// are formatted with integer math only (money.ts). A system/mint account's negative
// balance (allow_negative) is shown honestly with a leading '-' and a flag, never hidden.

import { useCallback, useEffect, useState } from "react";
import type { ApiClient } from "../api/client";
import { ApiError, type Account } from "../api/types";
import { formatMinor } from "../money";

interface Row {
  account: Account;
  amount: number | null; // minor units; null while loading or if the balance read failed
  dropped: boolean; // the account 404'd on the balance read — drop it quietly, not a banner
}

export function AccountsPane({
  client,
  selectedId,
  onSelect,
  onReauth,
}: {
  client: ApiClient;
  selectedId: string | null;
  onSelect: (id: string) => void;
  onReauth: () => void;
}) {
  const [rows, setRows] = useState<Row[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const accounts = await client.listAccounts();
      // Fetch balances concurrently; a single account 404 drops that row, not the view.
      const settled = await Promise.all(
        accounts.map(async (account): Promise<Row> => {
          try {
            const bal = await client.getBalance(account.id);
            return { account, amount: bal.amount, dropped: false };
          } catch (e) {
            if (e instanceof ApiError && e.code === "unauthenticated") throw e;
            if (e instanceof ApiError && e.code === "account_not_found") {
              return { account, amount: null, dropped: true };
            }
            return { account, amount: null, dropped: false };
          }
        }),
      );
      setRows(settled.filter((r) => !r.dropped));
    } catch (e) {
      if (e instanceof ApiError && e.code === "unauthenticated") {
        onReauth();
        return;
      }
      const msg = e instanceof ApiError ? e.userMessage : "Could not load your accounts.";
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, [client, onReauth]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <div className="pane">
      <div className="pane-head">
        <h2>Balances</h2>
        <button className="link-button" onClick={() => void load()} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </button>
      </div>

      {error !== null && <p className="error-banner">{error}</p>}
      {!loading && error === null && rows.length === 0 && (
        <p className="empty">You have no accounts yet.</p>
      )}

      <ul className="account-list">
        {rows.map((r) => (
          <AccountRow
            key={r.account.id}
            row={r}
            selected={r.account.id === selectedId}
            onSelect={onSelect}
          />
        ))}
      </ul>
    </div>
  );
}

function AccountRow({
  row,
  selected,
  onSelect,
}: {
  row: Row;
  selected: boolean;
  onSelect: (id: string) => void;
}) {
  const { account, amount } = row;
  const negative = amount !== null && amount < 0;
  return (
    <li>
      <button
        className={`account-row${selected ? " selected" : ""}`}
        onClick={() => onSelect(account.id)}
      >
        <span className="acct-id">{account.id}</span>
        <span className={`acct-balance${negative ? " negative" : ""}`}>
          {amount === null ? "—" : formatMinor(amount, account.currency)}
        </span>
        <span className="acct-ccy">{account.currency}</span>
        {account.allow_negative && <span className="acct-flag" title="system/mint account">system</span>}
      </button>
    </li>
  );
}

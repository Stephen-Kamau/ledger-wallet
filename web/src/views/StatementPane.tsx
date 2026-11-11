// views/StatementPane.tsx — R21 statement view. GET /accounts/{id}/statement, keyset
// "load more": the previous response's next_after_seq is passed back as after_seq, never
// an offset or a page number (api/handlers.go parsePage; 20-web.md §2.2). Per D-D1 a
// non-owned read is 404, not 403 — there is no "not your account" state here.

import { useCallback, useEffect, useState } from "react";
import type { ApiClient } from "../api/client";
import { ApiError, type Posting } from "../api/types";
import { formatMinor } from "../money";

const PAGE_LIMIT = 50;

export function StatementPane({
  client,
  accountId,
  onReauth,
}: {
  client: ApiClient;
  accountId: string | null;
  onReauth: () => void;
}) {
  const [rows, setRows] = useState<Posting[]>([]);
  const [afterSeq, setAfterSeq] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // loadMore fetches the next keyset page starting at `cursor`. Called with 0 for the
  // first load after the account changes, then with the returned next_after_seq.
  const loadMore = useCallback(
    async (id: string, cursor: number, reset: boolean) => {
      setLoading(true);
      setError(null);
      try {
        const page = await client.getStatement(id, { limit: PAGE_LIMIT, afterSeq: cursor });
        setRows((prev) => (reset ? page.postings : [...prev, ...page.postings]));
        setAfterSeq(page.next_after_seq); // keyset cursor, never a page number
        setHasMore(page.has_more);
      } catch (e) {
        if (e instanceof ApiError && e.code === "unauthenticated") {
          onReauth();
          return;
        }
        // A 404 here means the account does not exist OR is not the principal's (D-D1).
        if (e instanceof ApiError && e.code === "account_not_found") {
          setError("That account is not available.");
          return;
        }
        setError(e instanceof ApiError ? e.userMessage : "Could not load the statement.");
      } finally {
        setLoading(false);
      }
    },
    [client, onReauth],
  );

  // Reset and first-load whenever the selected account changes.
  useEffect(() => {
    setRows([]);
    setAfterSeq(0);
    setHasMore(false);
    setError(null);
    if (accountId !== null) void loadMore(accountId, 0, true);
  }, [accountId, loadMore]);

  if (accountId === null) {
    return (
      <div className="pane">
        <h2>Statement</h2>
        <p className="empty">Select an account to see its statement.</p>
      </div>
    );
  }

  return (
    <div className="pane">
      <h2>Statement</h2>
      <p className="subhead">{accountId}</p>

      {error !== null && <p className="error-banner">{error}</p>}

      <table className="statement">
        <thead>
          <tr>
            <th>seq</th>
            <th>transfer</th>
            <th className="num">amount</th>
            <th>when</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((p) => (
            <PostingRow key={p.id ?? `${p.seq}`} posting={p} />
          ))}
        </tbody>
      </table>

      {rows.length === 0 && error === null && !loading && (
        <p className="empty">No postings yet.</p>
      )}

      {hasMore && (
        <button
          className="load-more"
          disabled={loading}
          onClick={() => void loadMore(accountId, afterSeq, false)}
        >
          {loading ? "Loading…" : "Load more"}
        </button>
      )}
    </div>
  );
}

function PostingRow({ posting }: { posting: Posting }) {
  const credit = posting.amount >= 0; // debits negative/red, credits positive/green
  const ccy = posting.currency ?? "";
  return (
    <tr>
      <td className="mono">{posting.seq}</td>
      <td className="mono truncate" title={posting.transfer_id ?? ""}>
        {posting.transfer_id ?? "—"}
      </td>
      <td className={`num${credit ? " credit" : " debit"}`}>
        {formatMinor(posting.amount, ccy)}
      </td>
      <td>{posting.created_at ?? "—"}</td>
    </tr>
  );
}

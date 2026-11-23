// views/StatementPane.tsx — R21 statement view, plus entry reversal (R5 append-only
// correction). GET /accounts/{id}/statement, keyset "load more": the previous
// response's next_after_seq is passed back as after_seq, never an offset or a page
// number (api/handlers.go parsePage; 20-web.md §2.2). Per D-D1 a non-owned READ is 404,
// not 403 — so "not your account" is a state of the reverse ACTION, never of the list.
//
// Reversal lives here rather than in its own pane because the thing being reversed is a
// row of this table: the action, its refusal, and the correction it produces all have to
// land where the user is looking. The two pieces of judgement are extracted and tested:
// ./reversalErrors (code -> copy, placement, next action) and ./reversalIndex (which
// rows are corrections, which have been corrected, and where Reverse is offered).

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { newIdempotencyKey, type ApiClient } from "../api/client";
import { ApiError, type Posting } from "../api/types";
import { formatMinor } from "../money";
import { mapReversalError, unconfirmedOutcome, type MappedReversalError } from "./reversalErrors";
import { ReversalIntents } from "./reversalIntent";
import {
  buildReversalIndex,
  canReverse,
  localReversalFrom,
  reversalDetail,
  reversalLabel,
  reversalStateFor,
  type LocalReversal,
  type RowReversalState,
} from "./reversalIndex";

const PAGE_LIMIT = 50;

// A row-level reversal failure, pinned to the entry the user acted on.
interface RowError {
  entryId: string;
  mapped: MappedReversalError;
}

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

  // Reversal state. `localReversals` is the session overlay described in
  // ./reversalIndex: what we learned from a POST response before the statement
  // projection reported it. `confirming` is the two-step guard — reversal moves money,
  // so it is never one click, and never a browser confirm() either.
  const [localReversals, setLocalReversals] = useState<LocalReversal[]>([]);
  const [confirming, setConfirming] = useState<string | null>(null);
  const [reversing, setReversing] = useState<string | null>(null);
  const [rowError, setRowError] = useState<RowError | null>(null);
  const [paneError, setPaneError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  // One idempotency key per reversal intent, reused across retries (C1). A ref, not
  // state: changing it must never re-render, and a re-render must never reset it.
  const intents = useRef(new ReversalIntents(newIdempotencyKey));

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

  // Reset and first-load whenever the selected account changes. The session overlay is
  // cleared with it: a reversal link is only meaningful against the statement it came
  // from, and carrying it across accounts would mislabel an unrelated row.
  useEffect(() => {
    setRows([]);
    setAfterSeq(0);
    setHasMore(false);
    setError(null);
    setLocalReversals([]);
    setConfirming(null);
    setRowError(null);
    setPaneError(null);
    setNotice(null);
    intents.current.clear();
    if (accountId !== null) void loadMore(accountId, 0, true);
  }, [accountId, loadMore]);

  const index = useMemo(() => buildReversalIndex(rows, localReversals), [rows, localReversals]);

  // refresh reloads from the first keyset page. Reversal appends a row somewhere after
  // the entry it corrects, and the only way to see it is to re-read the statement; we
  // deliberately go back to page 1 rather than guess where the correction landed. The
  // overlay survives the reload, so the correction is labelled the moment it appears.
  const refresh = useCallback(() => {
    if (accountId === null) return;
    setRowError(null);
    setPaneError(null);
    void loadMore(accountId, 0, true);
  }, [accountId, loadMore]);

  const reverse = useCallback(
    async (entryId: string) => {
      setConfirming(null);
      setReversing(entryId);
      setRowError(null);
      setPaneError(null);
      setNotice(null);
      try {
        // One idempotency key per reversal INTENT (R4), held in `intents` and REUSED by
        // every retry of this entry, so a retried reverse replays the original correction
        // instead of posting a second one. See ./reversalIntent for why this is not a
        // fresh key per call.
        const body = await client.reverseEntry(entryId, intents.current.keyFor(entryId));
        setLocalReversals((prev) => [
          ...prev,
          localReversalFrom(body.reverses_entry_id ?? entryId, body.transfer_id),
        ]);
        setNotice(
          body.replayed
            ? "This entry was already corrected by an earlier identical request; the existing reversal is shown."
            : "Reversed. The correction has been posted to this statement as a new entry.",
        );
        // The intent is closed: the correction is confirmed, so a later reverse of this
        // entry is a new intent and must not replay this key.
        intents.current.close(entryId);
        if (accountId !== null) void loadMore(accountId, 0, true);
      } catch (e) {
        if (!(e instanceof ApiError)) {
          // A transport failure is NOT evidence that nothing posted — the request may
          // have reached the server and committed (C2). The intent stays OPEN so a retry
          // reuses the key and cannot double-post.
          setRowError({
            entryId,
            mapped: {
              ...unconfirmedOutcome(""),
              message:
                "Could not reach the server, so we cannot confirm whether this entry was reversed. Refresh the statement to see what actually posted.",
            },
          });
          return;
        }
        const mapped = mapReversalError(e);
        if (mapped.reauth) {
          onReauth();
          return;
        }
        if (mapped.placement === "pane") setPaneError(mapped.message);
        else setRowError({ entryId, mapped });
      } finally {
        setReversing(null);
      }
    },
    [client, accountId, loadMore, onReauth],
  );

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
      {paneError !== null && (
        <p className="error-banner">
          {paneError}{" "}
          <button className="link-button" onClick={refresh}>
            Refresh statement
          </button>
        </p>
      )}
      {notice !== null && <p className="notice-banner">{notice}</p>}

      <table className="statement">
        <thead>
          <tr>
            <th>seq</th>
            <th>transfer</th>
            <th className="num">amount</th>
            <th>when</th>
            <th aria-label="reversal" />
          </tr>
        </thead>
        <tbody>
          {rows.map((p) => {
            const state = reversalStateFor(index, p);
            const key = p.id ?? `${p.seq}`;
            return (
              <PostingRow
                key={key}
                posting={p}
                state={state}
                confirming={p.id !== undefined && confirming === p.id}
                busy={p.id !== undefined && reversing === p.id}
                disabled={reversing !== null}
                rowError={p.id !== undefined && rowError?.entryId === p.id ? rowError.mapped : null}
                onAsk={() => p.id !== undefined && setConfirming(p.id)}
                onCancel={() => {
                  if (p.id !== undefined) intents.current.close(p.id);
                  setConfirming(null);
                }}
                onConfirm={() => p.id !== undefined && void reverse(p.id)}
                onRefresh={refresh}
                onDismissError={() => {
                  if (p.id !== undefined) intents.current.close(p.id);
                  setRowError(null);
                }}
              />
            );
          })}
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

function PostingRow({
  posting,
  state,
  confirming,
  busy,
  disabled,
  rowError,
  onAsk,
  onCancel,
  onConfirm,
  onRefresh,
  onDismissError,
}: {
  posting: Posting;
  state: RowReversalState;
  confirming: boolean;
  busy: boolean;
  disabled: boolean;
  rowError: MappedReversalError | null;
  onAsk: () => void;
  onCancel: () => void;
  onConfirm: () => void;
  onRefresh: () => void;
  onDismissError: () => void;
}) {
  const credit = posting.amount >= 0; // debits negative/red, credits positive/green
  const ccy = posting.currency ?? "";
  const label = reversalLabel(state);
  const detail = reversalDetail(state);
  const offerReverse = canReverse(state, posting);

  // A reversed original is struck through and dimmed: its amount is still the truth of
  // what was posted (append-only — we never rewrite it), but it no longer stands as the
  // net effect, and showing it at full weight would overstate the account's history.
  const rowClass = state.kind === "reversed" ? "reversed-row" : state.kind === "reversal" ? "reversal-row" : "";

  return (
    <>
      <tr className={rowClass}>
        <td className="mono">{posting.seq}</td>
        <td className="mono truncate" title={posting.transfer_id ?? ""}>
          {posting.transfer_id ?? "—"}
        </td>
        <td className={`num${credit ? " credit" : " debit"}`}>
          {formatMinor(posting.amount, ccy)}
        </td>
        <td>{posting.created_at ?? "—"}</td>
        <td className="row-actions">
          {label !== null && (
            <span className={`rev-badge rev-${state.kind}`} title={detail ?? ""}>
              {label}
            </span>
          )}
          {offerReverse && !confirming && (
            <button className="link-button" disabled={disabled} onClick={onAsk}>
              {busy ? "Reversing…" : "Reverse"}
            </button>
          )}
          {confirming && (
            <span className="confirm-inline">
              <span className="confirm-q">Reverse this entry?</span>
              <button className="link-button danger" disabled={busy} onClick={onConfirm}>
                {busy ? "Reversing…" : "Confirm"}
              </button>
              <button className="link-button" disabled={busy} onClick={onCancel}>
                Cancel
              </button>
            </span>
          )}
        </td>
      </tr>

      {/* The caption is what turns a correction from "an unexplained second transfer"
          into a reversal: it names the entry on the other end of the link. */}
      {detail !== null && (
        <tr className={`rev-caption ${rowClass}`}>
          <td />
          <td colSpan={4}>{detail}</td>
        </tr>
      )}

      {rowError !== null && (
        <tr className="rev-error">
          <td />
          <td colSpan={4}>
            <span className="field-error">{rowError.message}</span>{" "}
            {rowError.action === "refresh" && (
              <button className="link-button" onClick={onRefresh}>
                Refresh statement
              </button>
            )}
            {rowError.action === "retry" && (
              <button className="link-button" onClick={onConfirm}>
                Try again
              </button>
            )}
            <button className="link-button" onClick={onDismissError}>
              Dismiss
            </button>
          </td>
        </tr>
      )}
    </>
  );
}

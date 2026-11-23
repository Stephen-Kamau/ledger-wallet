// views/reversalIndex.ts — turns a page of statement postings into per-row reversal
// state. This is the module that satisfies the actual requirement: a compensating
// entry must READ as a reversal, not as an unexplained second transfer.
//
// R5 is append-only — a correction is a new posting, and the original is never edited
// or removed. So "reversed" is not a property of a row; it is a RELATIONSHIP between
// two rows, and the statement is honest only if both ends of it are labelled:
//
//   the original  -> "Reversed" + which entry reversed it, still showing its amount
//   the correction-> "Reversal of <entry>" + which entry it put right
//
// Two sources feed the index, in this order:
//
//   1. The server's link fields on Posting (reverses_entry_id, reversed_by_entry_id).
//      Authoritative, and the only source that survives a page reload. STUB CONTRACT:
//      see api/types.ts — dakari's endpoint is not on dev yet.
//   2. Reversals performed in THIS session. The POST response tells us the link before
//      any statement refetch does, and the server's statement projection may be read
//      from a replica that has not caught up. Without this overlay a user who clicks
//      Reverse watches a second transfer appear with no explanation, which is exactly
//      the failure mode the requirement names.
//
// The overlay only ever ADDS links the server has not reported yet; it never contradicts
// a server link, because (1) is applied last and wins on conflict.

import type { Posting } from "../api/types";

// A reversal this session performed: the entry the user reversed, and the compensating
// transfer the server created for it.
export interface LocalReversal {
  reversedEntryId: string;
  reversalTransferId: string;
}

export type RowReversalState =
  | { kind: "plain" }
  // This entry has been reversed by a later correction.
  | { kind: "reversed"; byEntryId?: string }
  // This entry IS the correction for an earlier one.
  | { kind: "reversal"; ofEntryId: string };

export type ReversalIndex = Map<string, RowReversalState>;

// buildReversalIndex keys by posting id. A row with no id cannot be linked in either
// direction and is simply absent from the map (callers read it as "plain").
export function buildReversalIndex(
  rows: readonly Posting[],
  local: readonly LocalReversal[] = [],
): ReversalIndex {
  const index: ReversalIndex = new Map();

  const locallyReversed = new Set(local.map((l) => l.reversedEntryId));
  // transfer_id -> the entry that transfer was created to reverse.
  const reversalTransfers = new Map(
    local.map((l) => [l.reversalTransferId, l.reversedEntryId] as const),
  );

  for (const row of rows) {
    if (row.id === undefined) continue;

    // (2) session overlay first, so (1) can override it below.
    if (row.transfer_id !== undefined && reversalTransfers.has(row.transfer_id)) {
      index.set(row.id, {
        kind: "reversal",
        ofEntryId: reversalTransfers.get(row.transfer_id)!,
      });
    } else if (locallyReversed.has(row.id)) {
      index.set(row.id, { kind: "reversed" });
    }

    // (1) server links win. Checked in this order because a row that is BOTH a
    // correction and itself corrected is a reversal of something first — that is the
    // identity the user needs to read the amount, and `reversed` is then carried by
    // the later row that points back at it.
    if (row.reverses_entry_id !== undefined) {
      index.set(row.id, { kind: "reversal", ofEntryId: row.reverses_entry_id });
    } else if (row.reversed_by_entry_id !== undefined) {
      index.set(row.id, { kind: "reversed", byEntryId: row.reversed_by_entry_id });
    }
  }

  // A correction that is on the page while the entry it reverses is NOT (the original
  // fell on an earlier keyset page) still reads as a reversal — it carries the id of
  // what it put right. We deliberately do not hide or merge it: append-only means both
  // rows exist, and a statement that quietly nets them out is lying about the ledger.
  return index;
}

export function reversalStateFor(index: ReversalIndex, row: Posting): RowReversalState {
  if (row.id === undefined) return { kind: "plain" };
  return index.get(row.id) ?? { kind: "plain" };
}

// canReverse decides whether to offer the action at all. Three rules, and no more —
// every other rejection belongs to the server, and inventing a fourth client-side rule
// would mean disabling a button for a reason the API does not actually hold.
//
//   - no id: nothing to address the call to.
//   - already reversed: the correction exists; offering Reverse again only buys a 409.
//   - a reversal row: reversing a correction is a real operation the server may well
//     allow, but it is not one this UI invites by default — the honest move there is a
//     fresh transfer the user can explain, not a double negative.
export function canReverse(state: RowReversalState, row: Posting): boolean {
  if (row.id === undefined) return false;
  return state.kind === "plain";
}

// reversalLabel is the short badge text for a row. Kept here, not in the component, so
// the wording is covered by a test rather than by a reviewer noticing it.
export function reversalLabel(state: RowReversalState): string | null {
  switch (state.kind) {
    case "reversed":
      return "Reversed";
    case "reversal":
      return "Reversal";
    case "plain":
      return null;
  }
}

// reversalDetail is the full sentence behind the badge (row title / caption). It names
// the OTHER entry, because "Reversal" on its own is the unexplained-second-transfer
// problem with a sticker on it.
export function reversalDetail(state: RowReversalState): string | null {
  switch (state.kind) {
    case "reversed":
      return state.byEntryId === undefined
        ? "This entry has been reversed by a later correction on this statement."
        : `This entry has been reversed by entry ${state.byEntryId}.`;
    case "reversal":
      return `This entry reverses entry ${state.ofEntryId}. It is the correction, not a new transfer.`;
    case "plain":
      return null;
  }
}

// localReversalFrom records what a successful POST /entries/{id}/reverse told us, for
// the session overlay above.
export function localReversalFrom(
  reversedEntryId: string,
  reversalTransferId: string,
): LocalReversal {
  return { reversedEntryId, reversalTransferId };
}

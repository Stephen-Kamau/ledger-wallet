// views/reversalErrors.ts — the ONLY place a reversal's stable error `code` becomes UI
// copy, placement and a next action (20-web.md §3). Same discipline as transferErrors:
// branch on the `code` string from api/errors.go, NEVER on HTTP status.
//
// Why a separate map from TRANSFER_ERRORS: the same code means a different thing here.
// `unauthorized` on a transfer is "you do not own the SOURCE account you typed"; on a
// reversal there is no form and no typed id — it is "this row is not yours". Copy that
// tells the user to check an account id they never entered would be a lie. The
// placements differ too (a row and a pane, not four fields and a form).
//
// Fail-closed: a reversal-reachable code with no row here is a failing test, not a
// silent generic banner. reversalErrors.test.ts asserts coverage.

import type { ApiError } from "../api/types";

// Placement. A reversal has no form, so there are only three places a message can go:
// against the row the user acted on, at the pane, or as a session-level re-auth.
export interface ReversalPlacement {
  row?: boolean;
  pane?: boolean;
  reauth?: boolean;
}

// The next thing the user can DO. This is what keeps a typed failure from being a raw
// error body with better wording: every rejection names the move that resolves it.
export type ReversalAction = "refresh" | "reauth" | "retry" | "none";

interface ReversalErrorSpec {
  msg: string;
  at: ReversalPlacement;
  action: ReversalAction;
}

// STUB CONTRACT HEDGE. dakari's endpoint is not on dev yet, so the exact spelling of
// the already-reversed code is not yet fixed; the burst brief names the condition
// ("409 already-reversed") but not the string. Canonical spelling is `already_reversed`;
// the aliases below fold the other plausible spellings onto it so a naming near-miss
// degrades to the right message instead of the generic bucket. DELETE the aliases once
// `POST /entries/{id}/reverse` lands and the real code is read from api/errors.go.
export const REVERSAL_CODE_ALIASES: Record<string, string> = {
  entry_already_reversed: "already_reversed",
  posting_already_reversed: "already_reversed",
  entry_not_found: "entry_not_found",
  posting_not_found: "entry_not_found",
};

export function canonicalReversalCode(code: string): string {
  return REVERSAL_CODE_ALIASES[code] ?? code;
}

export const REVERSAL_ERRORS: Record<string, ReversalErrorSpec> = {
  // 409 — the correction already exists. This is the one rejection that is not a
  // failure at all: the ledger is in the state the user wanted. Say so, and send them
  // to the correction rather than inviting a pointless retry.
  already_reversed: {
    at: { row: true },
    action: "refresh",
    msg: "This entry has already been reversed. The correction is already on this statement — refresh to see it. Nothing was posted twice.",
  },

  // 403 — we know who you are; this row is simply not yours to correct. NOT re-auth:
  // signing in again cannot change the answer (§3.2).
  unauthorized: {
    at: { row: true },
    action: "none",
    msg: "This entry belongs to an account you do not own, so you cannot reverse it. Ask that account's owner to post the correction.",
  },

  // 404 — the entry is not on this account any more (or never was). The statement in
  // front of the user is stale; refreshing is the move.
  entry_not_found: {
    at: { row: true },
    action: "refresh",
    msg: "That entry is no longer on this account. Refresh the statement to see what is actually there.",
  },
  account_not_found: {
    at: { pane: true },
    action: "refresh",
    msg: "That account is not available any more. Refresh, or pick another account.",
  },

  // 409 / 422 — the compensating transfer was itself rejected by an invariant. The
  // original entry stands, unreversed; say which, because "reversal failed" alone
  // leaves the user guessing whether money moved.
  insufficient_funds: {
    at: { row: true },
    action: "none",
    msg: "Reversing this entry would overdraw the account the money has to come back out of. The entry was not reversed and no money moved.",
  },
  currency_mismatch: {
    at: { row: true },
    action: "none",
    msg: "This entry's accounts no longer share one currency, so it cannot be reversed automatically. The entry was not reversed.",
  },
  unbalanced: {
    at: { row: true },
    action: "retry",
    msg: "The correction did not balance, so nothing was posted. Try again.",
  },
  invalid_account: {
    at: { row: true },
    action: "none",
    msg: "One of the accounts on this entry is no longer valid, so the correction cannot be posted. The entry was not reversed.",
  },

  // 409 — our own idempotency key collided with a different intent. A client bug, but
  // the user can act on it: start the reversal again and a fresh key is minted.
  idempotency_conflict: {
    at: { row: true },
    action: "retry",
    msg: "This reversal clashed with an earlier one under the same reference. Nothing was posted — start the reversal again.",
  },

  // 400 — our bugs. Should not reach a user; still typed, still actionable.
  idempotency_key_required: {
    at: { row: true },
    action: "retry",
    msg: "Internal error: the reversal reference was missing, so nothing was posted. Try again.",
  },
  bad_request: {
    at: { row: true },
    action: "retry",
    msg: "The reversal request was malformed, so nothing was posted. Try again.",
  },
  unprocessable_entity: {
    at: { row: true },
    action: "retry",
    msg: "This entry cannot be reversed as it stands. Nothing was posted.",
  },

  // 401 — the identity is gone. Session-level, never inline (§3.2).
  unauthenticated: {
    at: { reauth: true },
    action: "reauth",
    msg: "Your session has expired. Please sign in again — the entry was not reversed.",
  },
};

// What the StatementPane renders: one message, where it goes, and the move it offers.
export interface MappedReversalError {
  message: string;
  placement: "row" | "pane";
  action: ReversalAction;
  reauth: boolean;
}

// mapReversalError turns a thrown ApiError into placed, actionable copy. Only an
// UNMAPPED code reaches the generic sentence, and even that states the financial fact
// that matters — the entry was NOT reversed — and quotes the request id. It never
// surfaces `e.userMessage` raw: a server string is a developer's sentence, and the
// whole point of this map is that the user reads ours.
export function mapReversalError(e: ApiError): MappedReversalError {
  const spec = REVERSAL_ERRORS[canonicalReversalCode(e.code)];
  if (spec === undefined) {
    const idPart = e.requestId ? ` (id ${e.requestId})` : "";
    return {
      message: `Something unexpected happened on our side${idPart}. This entry was not reversed.`,
      placement: "row",
      action: "retry",
      reauth: false,
    };
  }
  return {
    message: spec.msg,
    placement: spec.at.pane ? "pane" : "row",
    action: spec.action,
    reauth: spec.at.reauth === true,
  };
}

// Codes POST /entries/{id}/reverse can return, used by the coverage test so no
// reversal-reachable code falls through to the generic bucket. `internal`/`unknown`
// are deliberately excluded: they ARE the generic bucket.
export const REVERSAL_REACHABLE_CODES: readonly string[] = [
  "already_reversed",
  "unauthorized",
  "entry_not_found",
  "account_not_found",
  "insufficient_funds",
  "currency_mismatch",
  "unbalanced",
  "invalid_account",
  "idempotency_conflict",
  "idempotency_key_required",
  "bad_request",
  "unprocessable_entity",
  "unauthenticated",
];

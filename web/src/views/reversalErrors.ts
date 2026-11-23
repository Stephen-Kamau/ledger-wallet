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

  // 403 — we know who you are; you just cannot authorise THIS debit. NOT re-auth:
  // signing in again cannot change the answer (§3.2).
  //
  // The reason is not that the row is not yours. On your own statement it IS yours.
  // The compensating entry debits where the money WENT (core/reversal.go:72-78 reads
  // `original.To` and compares its owner to the authorizer), so R7 binds on the
  // destination. Alice pays Bob -> only BOB can authorise giving it back. The earlier
  // copy said "this entry belongs to an account you do not own", which is false in the
  // exact case it renders in: Alice is looking at her own account.
  unauthorized: {
    at: { row: true },
    action: "none",
    msg: "The money from this entry went to an account you do not own, and reversing it would take the money back out of that account. Only its owner can do that — ask them to post the reversal.",
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

  // 409 / 422 — the compensating entry was rejected by an invariant, and NOTHING was
  // written. core.Reverse does the replay check, the already-reversed check, the
  // authorisation check, the balance check and the INSERT inside ONE transaction
  // (core/reversal.go:39 `l.store.InTx`), so a rejection rolls the whole thing back.
  //
  // This copy was wrong twice, in opposite directions, and both are worth stating.
  // First it said only "the entry was not reversed", which reads as "try again later".
  // Review (C5) showed reversal was claim-then-settle at the time, so a rejected leg
  // stranded a committed claim and `UNIQUE (original_transfer_id)` then blocked every
  // later attempt — the retry was a promise the schema had revoked. The copy was
  // changed to say the entry could never be reversed again.
  //
  // core.Ledger.Reverse then landed atomic, and the reversals table is deliberately
  // unwired, so there is no claim to strand: the entry IS still reversible and the
  // second wording became the lie. Verified, not taken on report — core/reversal.go
  // holds everything in one InTx, and `grep -rn reversals internal/sqlite/*.go core/*.go`
  // (non-test) is empty.
  //
  // So: no money moved, and a retry is honest once the underlying reason clears.
  insufficient_funds: {
    at: { row: true },
    action: "retry",
    msg: "Reversing this entry would overdraw the account the money has to come back out of. No money moved. You can try again once that account has the funds.",
  },
  currency_mismatch: {
    at: { row: true },
    action: "none",
    msg: "This entry's accounts no longer share one currency, so it cannot be reversed automatically. No money moved. Post a correcting transfer instead.",
  },
  // 422 — core refuses to compensate an entry whose own legs do not net to zero
  // (core/reversal.go:146-162). Posting a second unbalanced entry on top would be worse,
  // so there is nothing the user can retry into.
  unbalanced: {
    at: { row: true },
    action: "none",
    msg: "This entry's own legs do not add up, so it cannot be reversed automatically. No money moved. This one needs a manual correction.",
  },
  invalid_account: {
    at: { row: true },
    action: "none",
    msg: "One of the accounts on this entry is no longer valid, so the correction cannot be posted. No money moved. Post a correcting transfer instead.",
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

// UNCONFIRMED_OUTCOME is the honest answer when the client cannot know whether the
// correction posted. It is NOT a generic error message; it is a different claim.
//
// The earlier version of this said "This entry was not reversed." That is a statement of
// financial fact the client is not entitled to make. `internal` routes here deliberately,
// and api/errors.go `writeJSON` emits `500 internal` when the SUCCESS body fails to
// marshal — i.e. AFTER the ledger has committed. A panic after commit lands in the same
// place. So "was not reversed" is false in exactly the cases that reach this branch, and
// it was paired with a retry, which under C1's fresh key would have posted a second
// correction. Caught in review (C2, burst `reversals`).
//
// Refresh, never retry: the only safe next move is to go and look at what actually posted.
export function unconfirmedOutcome(requestId: string): MappedReversalError {
  const idPart = requestId ? ` (id ${requestId})` : "";
  return {
    message: `Something unexpected happened on our side${idPart}. We cannot confirm whether this entry was reversed — refresh the statement to see what actually posted.`,
    placement: "row",
    action: "refresh",
    reauth: false,
  };
}

// mapReversalError turns a thrown ApiError into placed, actionable copy. An UNMAPPED or
// `internal` code reaches the unconfirmed-outcome sentence above, which quotes the request
// id and makes no claim about whether money moved. It never surfaces `e.userMessage` raw:
// a server string is a developer's sentence, and the whole point of this map is that the
// user reads ours.
export function mapReversalError(e: ApiError): MappedReversalError {
  const spec = REVERSAL_ERRORS[canonicalReversalCode(e.code)];
  if (spec === undefined) return unconfirmedOutcome(e.requestId);
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

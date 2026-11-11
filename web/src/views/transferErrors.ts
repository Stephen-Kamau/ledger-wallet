// views/transferErrors.ts — the ONLY place a stable error `code` becomes UI copy and
// placement (20-web.md §3). The UI branches on the `code` string from api/errors.go,
// NEVER on HTTP status. Status is used only to decide re-auth vs inline (see §3.2).
//
// Fail-closed, mirroring the server's "every sentinel has a status row" test
// (api/errors.go coreErrorMap): a transfer-reachable code with no row here is a failing
// test, not a silent generic banner. transferErrors.test.ts asserts coverage.

import type { ApiError } from "../api/types";

export type Field = "from" | "to" | "amount" | "currency";

// Placement: inline at a field, at the form, or a session-level re-auth event.
export interface Placement {
  field?: Field;
  form?: boolean;
  reauth?: boolean;
}

interface ErrorSpec {
  msg: string;
  at: Placement;
}

// Every code POST /transfers can return (api/handlers.go + api/errors.go coreErrorMap +
// api/middleware.go), each mapped to one true sentence and one placement.
//
// Per D-D1 there is NO "403 not your account" state on a READ; `unauthorized` (403)
// survives here because an unauthorised DEBIT is a real transfer rejection (R7).
export const TRANSFER_ERRORS: Record<string, ErrorSpec> = {
  // 409 / 422 — inline business rejections. The form stays filled; the user corrects.
  insufficient_funds: {
    at: { field: "amount" },
    msg: "Not enough funds in the source account. The available balance is less than this transfer.",
  },
  currency_mismatch: {
    at: { field: "currency" },
    msg: "These accounts use different currencies. A transfer must be in one currency.",
  },
  unbalanced: {
    at: { form: true },
    msg: "This transfer did not balance. Please re-enter the amount and try again.",
  },
  invalid_account: {
    at: { field: "from" },
    msg: "One of the accounts is not valid for this transfer. Check the account ids.",
  },
  idempotency_conflict: {
    at: { form: true },
    msg: "This looks like a different transfer reusing an earlier reference. Start the transfer again.",
  },

  // 403 — we know who you are, you just cannot do this. NOT re-auth (§3.2).
  unauthorized: {
    at: { field: "from" },
    msg: "You do not own the source account, so you cannot transfer from it.",
  },
  forbidden_privileged_flag: {
    at: { form: true },
    msg: "You are not allowed to set that option.",
  },

  // 404 — unknown account id.
  account_not_found: {
    at: { field: "from" },
    msg: "That account does not exist. Check the account id.",
  },

  // 400 — client/self bugs. Should not normally reach the user.
  idempotency_key_required: {
    at: { form: true },
    msg: "Internal error: the transfer reference was missing. Please retry.",
  },
  bad_request: {
    at: { form: true },
    msg: "The request was malformed. Please retry.",
  },
  unprocessable_entity: {
    at: { form: true },
    msg: "A required field is missing.",
  },

  // 401 — the identity itself is gone. Session-level: re-auth, never inline (§3.2).
  unauthenticated: {
    at: { reauth: true },
    msg: "Your session has expired. Please sign in again.",
  },
};

// The shape mapTransferError returns: field-keyed messages, a form-level message, and a
// reauth flag the form reads to clear the token and route to sign-in.
export interface MappedTransferError {
  from?: string;
  to?: string;
  amount?: string;
  currency?: string;
  form?: string;
  reauth?: boolean;
}

// mapTransferError turns a thrown ApiError into placed UI copy. Only `internal`/`unknown`
// and any UNMAPPED code get the generic sentence — and even that quotes the request_id
// and states the financial fact that matters: the transfer was not made.
export function mapTransferError(e: ApiError): MappedTransferError {
  const spec = TRANSFER_ERRORS[e.code];
  if (spec === undefined) {
    const idPart = e.requestId ? ` (id ${e.requestId})` : "";
    return { form: `Something unexpected happened on our side${idPart}. Your transfer was not made.` };
  }
  if (spec.at.reauth) return { reauth: true, form: spec.msg };
  if (spec.at.field) return { [spec.at.field]: spec.msg };
  return { form: spec.msg };
}

// Codes this form can provoke, used by the coverage test to guarantee no transfer-
// reachable code falls through to the generic bucket. `internal`/`unknown` are
// deliberately excluded: they are the generic bucket.
export const TRANSFER_REACHABLE_CODES: readonly string[] = [
  "insufficient_funds",
  "currency_mismatch",
  "unbalanced",
  "invalid_account",
  "unauthorized",
  "forbidden_privileged_flag",
  "account_not_found",
  "idempotency_conflict",
  "idempotency_key_required",
  "bad_request",
  "unprocessable_entity",
  "unauthenticated",
];

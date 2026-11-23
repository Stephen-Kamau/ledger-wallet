// api/types.ts — mirrors the FROZEN HTTP contract (api/types.go, api/errors.go).
// Money is int64 minor units on the wire; currency an opaque label; timestamps RFC3339.

import type { Minor } from "../money";

// POST /accounts response and an element of GET /accounts (api/types.go accountResp).
export interface Account {
  id: string;
  owner: string;
  currency: string;
  allow_negative: boolean;
  created_at: string;
}

// GET /accounts list body (api/types.go accountsResp, D-C1).
export interface AccountsBody {
  accounts: Account[];
}

// GET /accounts/{id} body (api/types.go balanceResp). This is the ONLY read that
// returns a balance; it carries the currency so the UI needs no second call.
export interface BalanceBody {
  account_id: string;
  amount: Minor;
  currency: string;
}

// One leg in a transfer or statement response (api/types.go postingResp).
// id/transfer_id/currency/created_at are omitempty on the transfer legs and present
// on statement rows, so they are optional here.
export interface Posting {
  id?: string;
  transfer_id?: string;
  account_id: string;
  amount: Minor;
  currency?: string;
  seq: number;
  created_at?: string;

  // Reversal links (R5 append-only correction). Both are optional because a plain
  // posting carries neither, and because they are STUB CONTRACT pending dakari's
  // `POST /entries/{id}/reverse` landing — see ReversalBody below.
  //
  // reverses_entry_id: this posting IS the compensating leg for that earlier entry.
  // reversed_by_entry_id: this posting HAS BEEN reversed by that later entry.
  // A correction is a new entry, never an edit, so both directions are links and
  // neither mutates the row it points at.
  reverses_entry_id?: string;
  reversed_by_entry_id?: string;
}

// POST /transfers response (api/types.go transferResp). replayed selects 200 vs 201.
export interface TransferBody {
  transfer_id: string;
  from: string;
  to: string;
  amount: Minor;
  currency: string;
  replayed: boolean;
  created_at: string;
  postings: Posting[];
}

// POST /entries/{id}/reverse response — the compensating transfer (R5).
//
// STUB CONTRACT. dakari is building this endpoint in parallel and it is NOT on dev at
// the time this was written, nor documented in docs/architecture/10-api.md. The shape
// below is derived from the two things that ARE fixed: the endpoint path given in the
// burst brief, and the existing POST /transfers response (transferResp), which this
// mirrors because a reversal IS a transfer — the only honest difference is the link
// back to the entry it compensates. `replayed` is carried for the same reason
// POST /transfers carries it: the reverse is sent with an Idempotency-Key (R4), so a
// retried reverse must return the original correction rather than post a second one.
export interface ReversalBody {
  transfer_id: string; // the compensating transfer this reversal created
  reverses_entry_id: string; // the posting id that was reversed
  amount: Minor;
  currency: string;
  replayed: boolean;
  created_at: string;
  postings: Posting[];
}

// GET /accounts/{id}/statement body (api/types.go statementResp), keyset-paged.
export interface StatementBody {
  account_id: string;
  postings: Posting[];
  next_after_seq: number;
  has_more: boolean;
}

// Request bodies.
export interface CreateAccountReq {
  currency: string;
  allow_negative?: boolean;
}
export interface TransferReq {
  from: string;
  to: string;
  amount: Minor;
  currency: string;
}

// Keyset page cursor (api/handlers.go parsePage: ?limit= & ?after_seq=).
export interface Page {
  limit: number;
  afterSeq: number;
}

// The one error body, every endpoint (api/errors.go errorBody).
export interface ApiErrorBody {
  error: { code: string; message: string; request_id: string };
}

// A typed throwable the UI branches on by `code`, never by HTTP status (20-web.md §3).
export class ApiError extends Error {
  readonly status: number;
  readonly code: string; // stable string from api/errors.go — the thing the UI keys off
  readonly userMessage: string; // server `message`
  readonly requestId: string; // quote in a support/debug affordance

  constructor(status: number, code: string, userMessage: string, requestId: string) {
    super(`${code} (${status})`);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.userMessage = userMessage;
    this.requestId = requestId;
  }
}

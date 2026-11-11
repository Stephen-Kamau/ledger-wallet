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

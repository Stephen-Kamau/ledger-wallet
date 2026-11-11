// api/client.ts — the ONLY module that touches fetch(). Nothing else in the UI builds
// a URL, sets a header, or parses an error body. This is the mirror of the server's
// "one errors.Is map" discipline (api/errors.go): the §3 error contract is enforced in
// exactly one place.

import {
  ApiError,
  type Account,
  type AccountsBody,
  type ApiErrorBody,
  type BalanceBody,
  type CreateAccountReq,
  type Page,
  type StatementBody,
  type TransferBody,
  type TransferReq,
} from "./types";

export interface ApiClient {
  listAccounts(): Promise<Account[]>; // GET /accounts (R20, D-C1)
  getBalance(id: string): Promise<BalanceBody>; // GET /accounts/{id}
  getStatement(id: string, page: Page): Promise<StatementBody>; // GET /accounts/{id}/statement (R21)
  createTransfer(req: TransferReq, idemKey: string): Promise<TransferBody>; // POST /transfers (R22)
  createAccount(req: CreateAccountReq): Promise<Account>; // POST /accounts
}

// newIdempotencyKey generates one key per transfer INTENT (api/handlers.go requires
// the Idempotency-Key header; R4). crypto.randomUUID is available in every browser the
// toy targets and under Node 22 / jsdom.
export function newIdempotencyKey(): string {
  return crypto.randomUUID();
}

interface CallOpts {
  body?: unknown;
  idemKey?: string;
  query?: Record<string, string | number>;
}

export function makeClient(baseUrl: string, getToken: () => string | null): ApiClient {
  async function call<T>(method: string, path: string, opts: CallOpts = {}): Promise<T> {
    const token = getToken();

    let url = baseUrl + path;
    if (opts.query) {
      const qs = new URLSearchParams();
      for (const [k, v] of Object.entries(opts.query)) qs.set(k, String(v));
      url += `?${qs.toString()}`;
    }

    const headers: Record<string, string> = {};
    if (token) headers["Authorization"] = `Bearer ${token}`; // api/middleware.go §2
    if (opts.body !== undefined) headers["Content-Type"] = "application/json";
    if (opts.idemKey) headers["Idempotency-Key"] = opts.idemKey; // api/handlers.go §5

    const res = await fetch(url, {
      method,
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    });

    if (!res.ok) {
      // Parse the ONE error shape (api/errors.go errorBody). If the body is not even
      // JSON, still surface a TYPED error rather than leak a raw status — but never
      // pretend we know the code.
      const parsed = (await res.json().catch(() => null)) as ApiErrorBody | null;
      const detail = parsed?.error;
      throw new ApiError(
        res.status,
        detail?.code ?? "unknown",
        detail?.message ?? "The server returned an unreadable error.",
        detail?.request_id ?? "",
      );
    }

    // 204/empty-body success is not part of this contract; every 2xx carries JSON.
    return (await res.json()) as T;
  }

  return {
    listAccounts: async () => {
      const body = await call<AccountsBody>("GET", "/accounts");
      return body.accounts;
    },
    getBalance: (id) => call<BalanceBody>("GET", `/accounts/${encodeURIComponent(id)}`),
    getStatement: (id, page) =>
      call<StatementBody>("GET", `/accounts/${encodeURIComponent(id)}/statement`, {
        query: { limit: page.limit, after_seq: page.afterSeq },
      }),
    createTransfer: (req, idemKey) =>
      call<TransferBody>("POST", "/transfers", { body: req, idemKey }),
    createAccount: (req) => call<Account>("POST", "/accounts", { body: req }),
  };
}

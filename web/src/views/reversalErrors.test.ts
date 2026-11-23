import { describe, expect, it } from "vitest";
import { ApiError } from "../api/types";
import {
  canonicalReversalCode,
  mapReversalError,
  unconfirmedOutcome,
  REVERSAL_CODE_ALIASES,
  REVERSAL_ERRORS,
  REVERSAL_REACHABLE_CODES,
} from "./reversalErrors";

function err(code: string, status = 409): ApiError {
  return new ApiError(status, code, "raw server sentence", "req_rev");
}

describe("reversal error map — fail-closed coverage", () => {
  it("maps every reversal-reachable code (none falls to the generic bucket)", () => {
    for (const code of REVERSAL_REACHABLE_CODES) {
      expect(REVERSAL_ERRORS[code], `missing mapping for ${code}`).toBeDefined();
    }
  });

  it("every alias resolves to a code that is actually mapped", () => {
    for (const [alias, canonical] of Object.entries(REVERSAL_CODE_ALIASES)) {
      expect(canonicalReversalCode(alias)).toBe(canonical);
      expect(REVERSAL_ERRORS[canonical], `alias ${alias} -> unmapped ${canonical}`).toBeDefined();
    }
  });

  it("every mapped message is a sentence, not a code echo", () => {
    for (const [code, spec] of Object.entries(REVERSAL_ERRORS)) {
      expect(spec.msg.length, code).toBeGreaterThan(20);
      expect(spec.msg, code).not.toContain("_");
      expect(spec.msg.endsWith("."), code).toBe(true);
    }
  });
});

describe("the two failures the brief names", () => {
  it("409 already-reversed says the correction exists and offers a refresh, not a retry", () => {
    const m = mapReversalError(err("already_reversed", 409));
    expect(m.placement).toBe("row");
    expect(m.action).toBe("refresh");
    expect(m.reauth).toBe(false);
    expect(m.message).toContain("already been reversed");
    expect(m.message).toContain("Nothing was posted twice");
  });

  it("403 names the DESTINATION as the reason, not the row's own account", () => {
    // R7 binds on the account being debited, which is where the money WENT
    // (core/reversal.go:72-78). On her own statement Alice DOES own the row's account,
    // so "this entry belongs to an account you do not own" was false exactly here.
    const m = mapReversalError(err("unauthorized", 403));
    expect(m.placement).toBe("row");
    expect(m.action).toBe("none");
    expect(m.reauth).toBe(false);
    expect(m.message).toContain("went to an account you do not own");
    expect(m.message).not.toContain("This entry belongs to an account you do not own");
  });

  it("403 is NOT re-auth — signing in again cannot make the row yours", () => {
    expect(mapReversalError(err("unauthorized", 403)).reauth).toBe(false);
  });
});

describe("placement and session handling", () => {
  it("routes 401 unauthenticated to re-auth", () => {
    const m = mapReversalError(err("unauthenticated", 401));
    expect(m.reauth).toBe(true);
    expect(m.action).toBe("reauth");
  });

  it("places an account-level failure at the pane, not against a row", () => {
    expect(mapReversalError(err("account_not_found", 404)).placement).toBe("pane");
  });

  it("places an entry-level failure against the row", () => {
    expect(mapReversalError(err("entry_not_found", 404)).placement).toBe("row");
  });
});

describe("an unknown outcome is reported as UNCONFIRMED, never as 'not reversed' (C2)", () => {
  it("quotes the request id and makes no claim about whether money moved", () => {
    const m = mapReversalError(err("some_new_code", 500));
    expect(m.message).toContain("req_rev");
    expect(m.message).toContain("cannot confirm");
    expect(m.message).not.toContain("was not reversed");
  });

  it("sends the user to REFRESH, not retry — a retry could double-post", () => {
    expect(mapReversalError(err("some_new_code", 500)).action).toBe("refresh");
  });

  it("treats `internal` the same way: it can be emitted AFTER the ledger commits", () => {
    const m = mapReversalError(err("internal", 500));
    expect(m.message).toContain("cannot confirm");
    expect(m.action).toBe("refresh");
  });

  it("still works with no request id", () => {
    expect(unconfirmedOutcome("").message).toContain("cannot confirm");
  });

  it("a rejected compensating entry says no money moved and never claims a dead end", () => {
    // core.Reverse is ONE transaction (core/reversal.go:39), so a rejection writes
    // nothing and the entry stays reversible. Claiming "cannot be reversed again" was
    // true only while reversal was claim-then-settle; it is now false.
    for (const code of ["insufficient_funds", "currency_mismatch", "invalid_account"]) {
      const m = mapReversalError(err(code));
      expect(m.message, code).toMatch(/[Nn]o money moved/);
      expect(m.message, code).not.toContain("cannot be reversed again");
    }
  });

  it("insufficient_funds offers a retry, because the funds can arrive later", () => {
    expect(mapReversalError(err("insufficient_funds", 409)).action).toBe("retry");
  });

  it("never leaks the raw server sentence for a mapped code", () => {
    for (const code of REVERSAL_REACHABLE_CODES) {
      expect(mapReversalError(err(code)).message).not.toContain("raw server sentence");
    }
  });

  it("does not leak the raw server sentence for an unmapped code either", () => {
    expect(mapReversalError(err("mystery")).message).not.toContain("raw server sentence");
  });
});

describe("reversal copy differs from transfer copy where the meaning differs", () => {
  it("unauthorized does not tell the user to check an account id they never typed", () => {
    expect(mapReversalError(err("unauthorized", 403)).message).not.toContain("account id");
  });

  it("every code core.Reverse can return is mapped (verified against core/reversal.go)", () => {
    // ErrEntryNotFound, ErrAlreadyReversed, ErrUnauthorized, ErrInsufficientFunds,
    // ErrIdempotencyConflict, ErrUnbalanced, ErrAccountNotFound.
    for (const code of [
      "entry_not_found",
      "already_reversed",
      "unauthorized",
      "insufficient_funds",
      "idempotency_conflict",
      "unbalanced",
      "account_not_found",
    ]) {
      expect(REVERSAL_ERRORS[canonicalReversalCode(code)], code).toBeDefined();
    }
  });

  it("insufficient_funds states that no money moved", () => {
    const m = mapReversalError(err("insufficient_funds", 409));
    expect(m.message).toContain("No money moved");
  });

  it("maps entry_not_found, and does NOT alias transfer_not_found onto it", () => {
    // core emits entry_not_found. transfer_not_found is deliberately absent: an alias
    // that silently absorbed a code core never sends would hide a real seam mismatch.
    expect(REVERSAL_ERRORS["entry_not_found"]).toBeDefined();
    expect(canonicalReversalCode("transfer_not_found")).toBe("transfer_not_found");
    expect(REVERSAL_ERRORS[canonicalReversalCode("transfer_not_found")]).toBeUndefined();
  });
});

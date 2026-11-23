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

  it("403 not-your-account explains ownership and offers no retry", () => {
    const m = mapReversalError(err("unauthorized", 403));
    expect(m.placement).toBe("row");
    expect(m.action).toBe("none");
    expect(m.reauth).toBe(false);
    expect(m.message).toContain("do not own");
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

  it("a compensating-leg rejection says no money moved AND does not promise a retry (C5)", () => {
    // The claim row commits before the compensating transfer runs, so one of these
    // strands the claim and UNIQUE (original_transfer_id) blocks every later attempt.
    // Saying only "the entry was not reversed" reads as "try again later" — which the
    // schema has permanently revoked.
    for (const code of ["insufficient_funds", "currency_mismatch", "invalid_account"]) {
      const m = mapReversalError(err(code));
      expect(m.message, code).toMatch(/[Nn]o money moved/);
      expect(m.message, code).toContain("cannot be reversed again");
      expect(m.action, code).toBe("none");
    }
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

  it("insufficient_funds states that no money moved", () => {
    const m = mapReversalError(err("insufficient_funds", 409));
    expect(m.message).toContain("No money moved");
  });
});

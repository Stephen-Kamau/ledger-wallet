import { describe, expect, it } from "vitest";
import { ApiError } from "../api/types";
import {
  mapTransferError,
  TRANSFER_ERRORS,
  TRANSFER_REACHABLE_CODES,
} from "./transferErrors";

function err(code: string, status = 422): ApiError {
  return new ApiError(status, code, "server message", "req_test");
}

describe("transfer error map — fail-closed coverage", () => {
  it("maps every transfer-reachable code (no code falls to the generic bucket)", () => {
    for (const code of TRANSFER_REACHABLE_CODES) {
      expect(TRANSFER_ERRORS[code], `missing mapping for ${code}`).toBeDefined();
    }
  });

  it("places the five named business rejections at the right field/form", () => {
    expect(mapTransferError(err("insufficient_funds", 409))).toHaveProperty("amount");
    expect(mapTransferError(err("currency_mismatch", 422))).toHaveProperty("currency");
    expect(mapTransferError(err("unauthorized", 403))).toHaveProperty("from");
    expect(mapTransferError(err("account_not_found", 404))).toHaveProperty("from");
    expect(mapTransferError(err("unbalanced", 422))).toHaveProperty("form");
  });

  it("routes 401 unauthenticated to re-auth, not an inline field", () => {
    const mapped = mapTransferError(err("unauthenticated", 401));
    expect(mapped.reauth).toBe(true);
  });

  it("does NOT re-auth on a 403 unauthorized (we know who you are; inline instead)", () => {
    const mapped = mapTransferError(err("unauthorized", 403));
    expect(mapped.reauth).toBeUndefined();
    expect(mapped.from).toBeDefined();
  });

  it("gives an unknown code the generic sentence quoting the request id, not a lie", () => {
    const mapped = mapTransferError(err("some_new_code", 500));
    expect(mapped.form).toContain("req_test");
    expect(mapped.form).toContain("not made");
  });

  it("never emits a generic message for a mapped business code", () => {
    const mapped = mapTransferError(err("insufficient_funds", 409));
    expect(mapped.form).toBeUndefined();
    expect(mapped.amount).not.toContain("unexpected");
  });
});

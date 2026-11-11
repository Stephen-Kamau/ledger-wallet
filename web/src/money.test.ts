import { describe, expect, it } from "vitest";
import { formatMinor, parseMinor, scaleFor } from "./money";

describe("formatMinor — integer-only, no float division", () => {
  it("formats a positive 2-digit amount", () => {
    expect(formatMinor(2510, "USD")).toBe("25.10");
  });
  it("renders zero as 0.00", () => {
    expect(formatMinor(0, "USD")).toBe("0.00");
  });
  it("renders a negative (system/mint) balance honestly with a leading -", () => {
    expect(formatMinor(-100000, "USD")).toBe("-1,000.00");
  });
  it("groups thousands", () => {
    expect(formatMinor(123456789, "USD")).toBe("1,234,567.89");
  });
  it("pads the fractional part", () => {
    expect(formatMinor(2500, "USD")).toBe("25.00");
    expect(formatMinor(2505, "USD")).toBe("25.05");
  });
  it("honours a scale-0 currency (JPY)", () => {
    expect(formatMinor(1500, "JPY")).toBe("1,500");
    expect(formatMinor(-7, "JPY")).toBe("-7");
  });
  it("defaults an unknown currency to 2 digits", () => {
    expect(formatMinor(199, "ZZZ")).toBe("1.99");
    expect(scaleFor("ZZZ")).toBe(2);
  });
  it("round-trips a large but safe integer exactly", () => {
    expect(formatMinor(900000000000000, "USD")).toBe("9,000,000,000,000.00");
  });
});

describe("parseMinor — string-based, no float drift", () => {
  it("parses whole major units", () => {
    expect(parseMinor("25", "USD")).toBe(2500);
  });
  it("parses the classic 25.10 without the *100 hazard", () => {
    expect(parseMinor("25.10", "USD")).toBe(2510);
  });
  it("pads a single fractional digit", () => {
    expect(parseMinor("25.5", "USD")).toBe(2550);
  });
  it("parses a negative amount", () => {
    expect(parseMinor("-3.20", "USD")).toBe(-320);
  });
  it("normalises -0 to 0", () => {
    expect(parseMinor("-0.00", "USD")).toBe(0);
    expect(parseMinor("0", "USD")).toBe(0);
  });
  it("rejects over-precise input rather than truncating money", () => {
    expect(parseMinor("25.001", "USD")).toBeNull();
  });
  it("rejects a fractional part for a scale-0 currency", () => {
    expect(parseMinor("5.0", "JPY")).toBeNull();
    expect(parseMinor("5", "JPY")).toBe(5);
  });
  it("rejects non-numeric and malformed input", () => {
    expect(parseMinor("abc", "USD")).toBeNull();
    expect(parseMinor("", "USD")).toBeNull();
    expect(parseMinor("1.2.3", "USD")).toBeNull();
    expect(parseMinor("1,000", "USD")).toBeNull();
  });
  it("is the exact inverse of formatMinor for representable values", () => {
    for (const v of [0, 1, 99, 100, 2510, -320, 123456789]) {
      expect(parseMinor(formatMinor(v, "USD").replace(/,/g, ""), "USD")).toBe(v);
    }
  });
});

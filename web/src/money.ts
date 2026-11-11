// money.ts — int64 minor units, end to end, no float drift (20-web.md §4).
//
// Money is an int64 count of minor units on the wire (Amara §2, Dakari §1). JS
// numbers are IEEE-754 doubles, exact for every integer up to 2^53 — far beyond any
// toy balance — so a minor-unit integer round-trips losslessly. We never divide a
// float to display it and never Number()-then-multiply to parse it.
//
// Boundary stated, not needed now: if a balance could exceed Number.MAX_SAFE_INTEGER
// minor units, Minor becomes bigint and the client parses with a custom reviver.

export type Minor = number; // int64 minor units. NEVER a display float.

// scale is minor-units-per-major as a power of ten. The toy assumes 2 for most
// currencies and 0 for JPY; a real system would read this per-currency from an
// authoritative source, not a front-end constant (20-web.md §5.4, flagged).
const SCALE: Record<string, number> = { USD: 2, GBP: 2, EUR: 2, JPY: 0 };

export function scaleFor(currency: string): number {
  const digits = SCALE[currency];
  return digits === undefined ? 2 : digits;
}

// formatMinor renders an int64 minor-unit amount as a human string using integer
// arithmetic only: integer division for the whole part, integer remainder for the
// fraction. A negative (system/mint) balance renders with a leading '-', never hidden.
export function formatMinor(amount: Minor, currency: string): string {
  if (!Number.isInteger(amount)) {
    // A non-integer minor amount is a contract violation upstream; surface it
    // rather than silently rounding money.
    return "NaN";
  }
  const digits = scaleFor(currency);
  const unit = 10 ** digits; // integer: 1, 100, ...
  const neg = amount < 0;
  const abs = Math.abs(amount);
  const whole = Math.trunc(abs / unit); // integer division (abs/unit is exact here)
  const frac = abs - whole * unit; // integer remainder — never a float modulo
  const fracStr = digits > 0 ? "." + String(frac).padStart(digits, "0") : "";
  return `${neg ? "-" : ""}${whole.toLocaleString("en-US")}${fracStr}`;
}

// parseMinor turns user input ("25", "25.00", "25.5", "-3.20") into exact int64 minor
// units, or null when the string does not represent an exact minor-unit amount for the
// currency. The amount is assembled by STRING manipulation (pad the fractional digits,
// concatenate, parse once as an integer) so "25.10" -> 2510 with no 25.1*100 hazard.
export function parseMinor(input: string, currency: string): Minor | null {
  const digits = scaleFor(currency);
  const m = input.trim().match(/^(-?)(\d+)(?:\.(\d+))?$/);
  if (m === null) return null;
  const sign = m[1] ?? "";
  const whole = m[2] ?? "";
  const frac = m[3] ?? "";
  if (frac.length > digits) return null; // more precision than the currency allows
  const scaled = whole + frac.padEnd(digits, "0"); // string concat, not float multiply
  const value = Number(scaled);
  if (!Number.isSafeInteger(value)) return null;
  if (value === 0) return 0; // normalise "-0" / "-0.00" to 0
  return sign === "-" ? -value : value;
}

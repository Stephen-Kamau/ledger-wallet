import { describe, expect, it } from "vitest";
import { ReversalIntents } from "./reversalIntent";

function counter() {
  let n = 0;
  return () => `key-${++n}`;
}

describe("one idempotency key per reversal INTENT, not per call (C1)", () => {
  it("returns the SAME key for every retry of the same entry", () => {
    const i = new ReversalIntents(counter());
    const first = i.keyFor("pst_1");
    expect(i.keyFor("pst_1")).toBe(first);
    expect(i.keyFor("pst_1")).toBe(first);
  });

  it("gives different entries different keys", () => {
    const i = new ReversalIntents(counter());
    expect(i.keyFor("pst_1")).not.toBe(i.keyFor("pst_2"));
  });

  it("keeps two concurrent intents apart across interleaved retries", () => {
    const i = new ReversalIntents(counter());
    const a1 = i.keyFor("pst_1");
    const b1 = i.keyFor("pst_2");
    expect(i.keyFor("pst_1")).toBe(a1);
    expect(i.keyFor("pst_2")).toBe(b1);
  });

  it("mints exactly one key per entry no matter how many attempts", () => {
    const mint = counter();
    const i = new ReversalIntents(mint);
    for (let n = 0; n < 5; n++) i.keyFor("pst_1");
    expect(i.keyFor("pst_1")).toBe("key-1");
  });

  it("a closed intent earns a NEW key, so a later reversal is not a replay", () => {
    const i = new ReversalIntents(counter());
    const first = i.keyFor("pst_1");
    i.close("pst_1");
    expect(i.keyFor("pst_1")).not.toBe(first);
  });

  it("clear drops every open intent", () => {
    const i = new ReversalIntents(counter());
    i.keyFor("pst_1");
    i.keyFor("pst_2");
    expect(i.openCount()).toBe(2);
    i.clear();
    expect(i.openCount()).toBe(0);
  });
});

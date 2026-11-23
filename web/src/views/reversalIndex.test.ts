import { describe, expect, it } from "vitest";
import type { Posting } from "../api/types";
import {
  buildReversalIndex,
  canReverse,
  localReversalFrom,
  reversalDetail,
  reversalLabel,
  reversalStateFor,
} from "./reversalIndex";

function p(over: Partial<Posting> & { seq: number }): Posting {
  return {
    id: `pst_${over.seq}`,
    transfer_id: `trf_${over.seq}`,
    account_id: "acc_a",
    amount: 1000,
    currency: "USD",
    created_at: "2026-11-23T00:00:00Z",
    ...over,
  };
}

describe("server link fields are the authoritative source", () => {
  it("labels the original as reversed and names the entry that reversed it", () => {
    const rows = [p({ seq: 1, reversed_by_entry_id: "pst_2" }), p({ seq: 2, reverses_entry_id: "pst_1" })];
    const ix = buildReversalIndex(rows);
    expect(reversalStateFor(ix, rows[0]!)).toEqual({ kind: "reversed", byEntryId: "pst_2" });
    expect(reversalStateFor(ix, rows[1]!)).toEqual({ kind: "reversal", ofEntryId: "pst_1" });
  });

  it("leaves an ordinary posting plain", () => {
    const rows = [p({ seq: 1 })];
    expect(reversalStateFor(buildReversalIndex(rows), rows[0]!)).toEqual({ kind: "plain" });
  });

  it("a correction whose original is on an earlier page still reads as a reversal", () => {
    const rows = [p({ seq: 9, reverses_entry_id: "pst_1" })];
    const st = reversalStateFor(buildReversalIndex(rows), rows[0]!);
    expect(st).toEqual({ kind: "reversal", ofEntryId: "pst_1" });
    expect(reversalDetail(st)).toContain("pst_1");
  });
});

describe("links derived from the rev_ id — this is what survives a reload", () => {
  // The live statement emits no reverses_entry_id / reversed_by_entry_id. Before this,
  // a correction read as an unexplained second transfer after any reload, because only
  // the session overlay linked it. core/reversal.go derives the id, so the page can too.
  const original = p({ seq: 4, id: "pst_4", transfer_id: "xfer_9" });
  const correction = p({ seq: 7, id: "pst_7", transfer_id: "rev_xfer_9", amount: -1000 });

  it("labels the correction, naming the POSTING it corrects, with no server fields", () => {
    const ix = buildReversalIndex([original, correction]);
    expect(reversalStateFor(ix, correction)).toEqual({ kind: "reversal", ofEntryId: "pst_4" });
  });

  it("labels the original as reversed, naming the correction's posting", () => {
    const ix = buildReversalIndex([original, correction]);
    expect(reversalStateFor(ix, original)).toEqual({ kind: "reversed", byEntryId: "pst_7" });
  });

  it("withholds Reverse on the reversed original after a reload", () => {
    const ix = buildReversalIndex([original, correction]);
    expect(canReverse(reversalStateFor(ix, original), original)).toBe(false);
  });

  it("still offers Reverse on the correction — reversing a reversal is allowed", () => {
    const ix = buildReversalIndex([original, correction]);
    expect(canReverse(reversalStateFor(ix, correction), correction)).toBe(true);
  });

  it("handles a reversal of a reversal (rev_rev_)", () => {
    const second = p({ seq: 9, id: "pst_9", transfer_id: "rev_rev_xfer_9" });
    const ix = buildReversalIndex([original, correction, second]);
    expect(reversalStateFor(ix, second)).toEqual({ kind: "reversal", ofEntryId: "pst_7" });
    // The correction is now itself corrected, but it is a reversal FIRST — that is the
    // identity the amount is read against.
    expect(reversalStateFor(ix, correction).kind).toBe("reversal");
  });

  it("names the transfer when the corrected posting is on an earlier keyset page", () => {
    const ix = buildReversalIndex([correction]);
    expect(reversalStateFor(ix, correction)).toEqual({ kind: "reversal", ofEntryId: "xfer_9" });
  });

  it("matches the counterpart leg on THIS account, not just the first leg found", () => {
    const otherLeg = p({ seq: 1, id: "pst_other", transfer_id: "xfer_9", account_id: "acc_b" });
    const ix = buildReversalIndex([otherLeg, original, correction]);
    expect(reversalStateFor(ix, correction)).toEqual({ kind: "reversal", ofEntryId: "pst_4" });
  });

  it("leaves an ordinary transfer untouched", () => {
    const plain = p({ seq: 2, id: "pst_2", transfer_id: "xfer_plain" });
    expect(reversalStateFor(buildReversalIndex([plain]), plain)).toEqual({ kind: "plain" });
  });

  it("an explicit server link still overrides the derived one", () => {
    const withField = p({ seq: 7, id: "pst_7", transfer_id: "rev_xfer_9", reverses_entry_id: "pst_explicit" });
    const ix = buildReversalIndex([original, withField]);
    expect(reversalStateFor(ix, withField)).toEqual({ kind: "reversal", ofEntryId: "pst_explicit" });
  });
});

describe("the session overlay fills the gap before the statement catches up", () => {
  it("marks a just-reversed entry as reversed even with no server field yet", () => {
    const rows = [p({ seq: 1 })];
    const ix = buildReversalIndex(rows, [localReversalFrom("pst_1", "trf_99")]);
    expect(reversalStateFor(ix, rows[0]!)).toEqual({ kind: "reversed" });
  });

  it("marks the compensating row as a reversal via its transfer id, not as a new transfer", () => {
    const rows = [p({ seq: 1 }), p({ seq: 2, id: "pst_2", transfer_id: "trf_99", amount: -1000 })];
    const ix = buildReversalIndex(rows, [localReversalFrom("pst_1", "trf_99")]);
    const st = reversalStateFor(ix, rows[1]!);
    expect(st).toEqual({ kind: "reversal", ofEntryId: "pst_1" });
    expect(reversalLabel(st)).toBe("Reversal");
  });

  it("a server link overrides a stale session overlay rather than contradicting it", () => {
    const rows = [p({ seq: 1, reverses_entry_id: "pst_0" })];
    const ix = buildReversalIndex(rows, [localReversalFrom("pst_1", "trf_other")]);
    expect(reversalStateFor(ix, rows[0]!)).toEqual({ kind: "reversal", ofEntryId: "pst_0" });
  });

  it("an empty overlay changes nothing", () => {
    const rows = [p({ seq: 1 })];
    expect(buildReversalIndex(rows, [])).toEqual(buildReversalIndex(rows));
  });
});

describe("rows with no id cannot be linked or actioned", () => {
  it("is absent from the index and reads as plain", () => {
    const rows = [p({ seq: 1, id: undefined })];
    const ix = buildReversalIndex(rows);
    expect(ix.size).toBe(0);
    expect(reversalStateFor(ix, rows[0]!)).toEqual({ kind: "plain" });
  });

  it("is not offered the reverse action", () => {
    expect(canReverse({ kind: "plain" }, p({ seq: 1, id: undefined }))).toBe(false);
  });
});

describe("canReverse offers the action exactly where it can succeed", () => {
  const row = p({ seq: 1 });
  it("offers it on an ordinary entry", () => {
    expect(canReverse({ kind: "plain" }, row)).toBe(true);
  });
  it("withholds it on an entry already reversed (that would only buy a 409)", () => {
    expect(canReverse({ kind: "reversed", byEntryId: "pst_2" }, row)).toBe(false);
  });
  it("OFFERS it on a correction row — core allows reversing a reversal", () => {
    // core/reversal.go:21-23: the derived id applied to a reversal's own id is "a
    // DISTINCT entry and a legitimate correction of a correction, so it is allowed".
    expect(canReverse({ kind: "reversal", ofEntryId: "pst_1" }, row)).toBe(true);
  });
});

describe("the labels say what the row IS, which is the whole requirement", () => {
  it("a plain row carries no badge and no caption", () => {
    expect(reversalLabel({ kind: "plain" })).toBeNull();
    expect(reversalDetail({ kind: "plain" })).toBeNull();
  });

  it("a correction's caption names the entry it puts right and denies being a transfer", () => {
    const d = reversalDetail({ kind: "reversal", ofEntryId: "pst_1" })!;
    expect(d).toContain("pst_1");
    expect(d).toContain("not a new transfer");
  });

  it("a reversed original says so even when the correction's id is not known yet", () => {
    expect(reversalDetail({ kind: "reversed" })).toContain("reversed by a later correction");
    expect(reversalDetail({ kind: "reversed", byEntryId: "pst_7" })).toContain("pst_7");
  });
});

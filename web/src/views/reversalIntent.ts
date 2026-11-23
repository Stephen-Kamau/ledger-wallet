// views/reversalIntent.ts — one idempotency key per reversal INTENT, reused across every
// retry of that intent (R4).
//
// WHAT ACTUALLY GUARANTEES NO DOUBLE REVERSAL — and it is not this file.
// `migrations/0004_reversals.sql` carries `UNIQUE (original_transfer_id)`. That constraint
// makes a second correction for the same entry structurally impossible, whatever key is
// sent; kofi's TestR18 proves it with deliberately distinct keys. Naming the key as the
// guard, as an earlier version of this comment did, credits the wrong mechanism.
//
// WHY THIS FILE STILL EARNS ITS KEEP. The first cut called
// `client.reverseEntry(entryId, newIdempotencyKey())`, minting a fresh key per CALL while
// claiming per-intent semantics. With the retry path live, a retried reverse is then a
// distinct intent at the API boundary: the database still refuses to double-post, but the
// client gets a conflict it caused itself rather than the replay R4 is supposed to give
// it. Reusing the key is what makes a retry idempotent at this layer instead of relying on
// a constraint two layers down to catch us. Raised as C1 and downgraded on that basis.
//
// The fix has to be a keyed store rather than a single key, because retries are per row
// and two rows can be mid-intent at once. It lives here, outside React, so the property
// is covered by a test instead of by a reviewer reading a component.

// An intent is opened when the user asks to reverse an entry, survives every retry, and
// is closed when the reversal is confirmed to have happened or the user walks away.
export class ReversalIntents {
  private keys = new Map<string, string>();

  constructor(private mint: () => string) {}

  // keyFor returns the key for this entry's OPEN intent, opening one if needed. Called
  // on every attempt, including retries — that is the whole point: the same entry gets
  // the same key until the intent is closed.
  keyFor(entryId: string): string {
    const existing = this.keys.get(entryId);
    if (existing !== undefined) return existing;
    const key = this.mint();
    this.keys.set(entryId, key);
    return key;
  }

  // close ends the intent. The NEXT reverse of this entry is a new intent and earns a new
  // key — otherwise a user who reversed, saw it land, and later reversed a re-created
  // entry would replay the first correction.
  close(entryId: string): void {
    this.keys.delete(entryId);
  }

  // clear drops every open intent — used when the selected account changes, since an
  // intent is only meaningful against the statement it was opened on.
  clear(): void {
    this.keys.clear();
  }

  openCount(): number {
    return this.keys.size;
  }
}

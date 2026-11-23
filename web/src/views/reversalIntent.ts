// views/reversalIntent.ts — one idempotency key per reversal INTENT, reused across every
// retry of that intent (R4).
//
// WHY THIS IS NOT INLINE IN THE COMPONENT. The first cut called
// `client.reverseEntry(entryId, newIdempotencyKey())` directly, minting a fresh key on
// every invocation, while the comment above it claimed per-intent semantics. The retry
// path is live — an unconfirmed outcome offers "Try again" — so against a backend that
// enforces R4 BY KEY the retry is a distinct intent and posts a SECOND compensating
// transfer. Money moves twice. Caught in review (C1, burst `reversals`).
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

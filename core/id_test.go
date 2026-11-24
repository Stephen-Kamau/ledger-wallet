package core

import (
	"strings"
	"testing"
)

// resolveEntry (reversal.go) tries FindTransferByID first and falls back to
// FindTransferByPostingID. Its comment justifies that ordering with "the ids are prefixed
// (`xfer_`, `post_`) so the two namespaces cannot collide anyway" - and until this file
// nothing asserted it. A silent ordered fallback resting on an unasserted invariant is the
// gap these tests close (zola's review of burst `reversals`, condition 2).
//
// Every id below is taken from the REAL minting path rather than written as a literal, so
// these stay true if a prefix is ever changed - a literal copied beside the subject would
// simply go stale and keep passing.
//
// Both are MUTATION-VERIFIED, not merely passing:
//   - minting postings with newID("xfer") instead of newID("post") (ledger.go:205) fails
//     TestIDNamespaces_PostingAndTransferIDsAreDisjoint;
//   - dropping the prefix from newID (id.go:18) fails TestNewID_TagsEveryNamespaceDistinctly.
// Both mutations were applied, run and reverted.
//
// A third test asserting a posting id does not resolve through FindTransferByID was written
// and then DELETED: the fake store cannot exhibit that failure, so it passed under every
// mutation and could not earn its keep. The property it aimed at is covered above.

// idPrefix returns the segment before the first separator - the namespace tag newID writes.
func idPrefix(id string) string {
	if i := strings.Index(id, "_"); i >= 0 {
		return id[:i]
	}
	return ""
}

// The invariant resolveEntry's ordering depends on: a posting id is never also a transfer
// id, so trying the transfer lookup first can never shadow a posting.
func TestIDNamespaces_PostingAndTransferIDsAreDisjoint(t *testing.T) {
	_, _, _, _, orig := reversible(t, 100, 30)

	if len(orig.Postings) == 0 {
		t.Fatal("a transfer with no postings cannot exercise the invariant")
	}
	for _, p := range orig.Postings {
		if p.ID == orig.ID {
			t.Fatalf("posting id %q equals its transfer id - resolveEntry's ordered fallback is unsafe", p.ID)
		}
		if idPrefix(p.ID) == idPrefix(orig.ID) {
			t.Fatalf("posting and transfer share namespace %q (posting %q, transfer %q)",
				idPrefix(p.ID), p.ID, orig.ID)
		}
	}
}

// Every id newID mints carries its namespace tag, across every prefix the ledger uses. This
// is what makes the disjointness above structural rather than incidental.
func TestNewID_TagsEveryNamespaceDistinctly(t *testing.T) {
	prefixes := []string{"acct", "xfer", "post", "hold"}
	seen := make(map[string]string, len(prefixes))

	for _, p := range prefixes {
		id := newID(p)
		if got := idPrefix(id); got != p {
			t.Fatalf("newID(%q) produced %q, whose namespace tag is %q", p, id, got)
		}
		if prev, dup := seen[idPrefix(id)]; dup {
			t.Fatalf("namespace %q minted by both %q and %q", idPrefix(id), prev, p)
		}
		seen[idPrefix(id)] = p
	}
}

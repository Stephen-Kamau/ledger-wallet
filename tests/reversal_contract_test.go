package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// The reversal link is a CROSS-LANGUAGE contract, and this file is the only thing that
// enforces it.
//
// The statement projection emits no reverses_entry_id / reversed_by_entry_id, so the web
// layer recovers both link directions by stripping core's derived-id prefix from
// `transfer_id` (web/src/views/reversalIndex.ts). That makes core.ReversalPrefix a value
// TWO languages depend on, and until this test it was spelled as a bare literal in four
// places with nothing binding them:
//
//	core/reversal.go                      the source of truth
//	core/reversal_test.go                 a copied literal
//	internal/sqlite/reversal_test.go      a copied literal
//	web/src/views/reversalIndex.ts        a copied literal
//
// The drift was SILENT and demonstrated by mutation: changing the constant in core failed
// exactly two Go tests, and both failed only because they hardcoded the literal too - not
// because they guarded the contract. A developer fixes those two literals, sees green, and
// ships a build where core emits one prefix while the web strips another. Statement links
// stop resolving with every Go and web test passing.
//
// The three Go sites now read the constant. This test is the fourth - the only one that
// cannot be satisfied by editing a literal, because it compares the two languages to each
// other.
//
// It FAILS CLOSED: if the declaration cannot be found at all - renamed, reformatted,
// deleted, file moved - that is a failure, not a skip. A contract test that quietly passes
// when it can no longer see its subject is the exact decoration this exists to replace.

// webReversalPrefixDecl matches `const REVERSAL_ID_PREFIX = "rev_";` tolerating quote style
// and spacing, but NOT tolerating absence.
var webReversalPrefixDecl = regexp.MustCompile(`const\s+REVERSAL_ID_PREFIX\s*(?::\s*string\s*)?=\s*["'` + "`" + `]([^"'` + "`" + `]*)["'` + "`" + `]`)

const webReversalIndexPath = "../web/src/views/reversalIndex.ts"

func TestReversalPrefix_WebAndCoreAgree(t *testing.T) {
	path, err := filepath.Abs(webReversalIndexPath)
	if err != nil {
		t.Fatalf("resolving %s: %v", webReversalIndexPath, err)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the web link module at %s: %v\n"+
			"This test pins core.ReversalPrefix to the TypeScript constant. If the module "+
			"moved, update webReversalIndexPath - do not delete the test.", path, err)
	}

	m := webReversalPrefixDecl.FindSubmatch(src)
	if m == nil {
		t.Fatalf("no `const REVERSAL_ID_PREFIX = \"...\"` declaration in %s.\n"+
			"The web layer derives the reversal link by stripping core's prefix from "+
			"transfer_id. If that constant was renamed or removed, this contract moved with "+
			"it and this test must be updated to follow - failing closed rather than "+
			"passing blind.", webReversalIndexPath)
	}

	if got := string(m[1]); got != core.ReversalPrefix {
		t.Fatalf("reversal prefix DRIFTED across languages:\n"+
			"  core.ReversalPrefix (core/reversal.go) = %q\n"+
			"  REVERSAL_ID_PREFIX  (%s) = %q\n"+
			"A compensating transfer's id is core's prefix + the original transfer id, and the "+
			"web layer recovers the link by stripping the same prefix. While these disagree, "+
			"corrections render as unexplained second transfers after a reload and the original "+
			"is never marked reversed - with every Go and web test green.",
			core.ReversalPrefix, webReversalIndexPath, got)
	}
}

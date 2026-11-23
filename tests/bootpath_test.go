package tests

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Stephen-Kamau/ledger-wallet/internal/sqlite"
	"github.com/Stephen-Kamau/ledger-wallet/migrations"
)

// The drift guard. `collision-v2` shipped a 500 because cmd/ledgerd booted the adapter's
// embeddedSchema, which migration 0002 had never reached: the live server had no
// currencies table, GET /currencies 500'd, and every migration-applied unit test stayed
// green throughout. A test that applies migrations itself cannot catch that class of bug
// -- it builds the very schema whose absence is the defect.
//
// So this file asserts against the REAL WIRED BOOT PATH: it builds the cmd/ledgerd
// binary, runs it, and inspects the database that binary actually created. Nothing is
// mocked and nothing is re-applied. cmd/ledgerd/wire_test.go makes the same check for
// the pre-0004 tables, but cmd/** is another operator's scope this burst, so the guard
// for migration 0004 lives here.

// bootTimeout bounds the wait for the booted server to create its schema. The build is
// NOT inside this budget -- buildLedgerd returns before the poll starts -- so this covers
// only process start plus applying four small migrations, which is sub-second in
// practice. Kept generous for a loaded machine, but short enough that the old wiring
// fails fast: at 120s a regression cost two minutes to report, which is the wrong
// trade for a guard whose whole job is to be noticed.
const bootTimeout = 30 * time.Second

// TestBootPath_ServerSchemaCarriesMigration0004 boots the real server binary and asserts
// the database it created carries everything 0004 defines -- the reversals table, its
// no-double-reversal constraint, and its indexes.
//
// WHY IT FAILS ON THE OLD WIRING: TestBootPath_EmbeddedSchemaLacksReversals below proves
// the adapter's embeddedSchema has no reversals table. So if cmd/ledgerd ever regresses
// to booting from embeddedSchema, the database this test inspects will not have the
// table and this test fails. The two tests are a pair; neither is sufficient alone.
func TestBootPath_ServerSchemaCarriesMigration0004(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "boot.db")
	bin := buildLedgerd(t, dir)

	// -addr 127.0.0.1:0 takes an ephemeral port: nothing connects to it, the point is
	// to exercise New() -- the wiring seam that applies the migrations -- in the real
	// process, not a test harness standing in for it.
	cmd := exec.Command(bin, "-db", dbPath, "-addr", "127.0.0.1:0")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ledgerd: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	db := waitForSchema(t, dbPath, "reversals", &out, cmd)
	ctx := context.Background()

	// Every object 0004 defines is present on the database the SERVER built.
	for _, obj := range []struct{ typ, name string }{
		{"table", "reversals"},
		{"index", "idx_reversals_reversal_transfer"},
		{"index", "idx_reversals_owner"},
		{"trigger", "reversals_no_repoint_original"},
		{"trigger", "reversals_settle_once"},
		{"trigger", "reversals_no_delete_settled"},
	} {
		var name string
		if err := db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_schema WHERE type = ? AND name = ?`, obj.typ, obj.name).Scan(&name); err != nil {
			t.Fatalf("the booted server's schema is missing %s %q -- migration 0004 did not "+
				"reach the wired boot path (schema drift: the server is not booting from "+
				"migrations.Apply): %v\nserver output:\n%s", obj.typ, obj.name, err, out.String())
		}
	}

	// The GUARANTEE itself reached the server, not merely a table of the right name.
	var ddl string
	if err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'reversals'`).Scan(&ddl); err != nil {
		t.Fatalf("read reversals DDL from the booted server: %v", err)
	}
	if !bytes.Contains([]byte(ddl), []byte("UNIQUE (original_transfer_id)")) {
		t.Fatalf("the booted server's reversals table carries no UNIQUE (original_transfer_id) -- "+
			"the no-double-reversal guarantee is not in the live schema:\n%s", ddl)
	}

	// Every migration is recorded as applied, 0004 included.
	var version string
	if err := db.QueryRowContext(ctx,
		`SELECT version FROM schema_migrations WHERE version = '0004'`).Scan(&version); err != nil {
		t.Fatalf("the booted server did not record migration 0004 as applied: %v", err)
	}

	// And the whole migration set reached it, not just the newest one -- the drift this
	// guards is a migration being SKIPPED, which an is-0004-there check alone would miss
	// if a later wiring applied only the last file.
	names, err := migrations.Names()
	if err != nil {
		t.Fatalf("migrations.Names: %v", err)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM schema_migrations`); n != int64(len(names)) {
		t.Fatalf("the booted server recorded %d migrations, want all %d", n, len(names))
	}
}

// TestBootPath_EmbeddedSchemaLacksReversals is the other half of the pair: it pins the
// fact that makes the test above a real regression guard. The adapter's standalone
// embeddedSchema does NOT carry the reversals table, so a server booted from it -- the
// old wiring -- cannot pass TestBootPath_ServerSchemaCarriesMigration0004.
//
// This is a deliberate assertion that the two schemas DIFFER, not a complaint about it.
// internal/sqlite is another operator's scope and embeddedSchema is adapter-test-only;
// the drift is harmless precisely because the wired path no longer boots from it. If
// someone later teaches embeddedSchema about reversals, this test fails and should be
// deleted along with the "fails on the old wiring" claim it backs -- the guard above
// would no longer be one, and that must not pass silently.
func TestBootPath_EmbeddedSchemaLacksReversals(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "embedded.db"))
	if err != nil {
		t.Fatalf("sqlite.Open (the OLD boot path): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var name string
	err = store.DB().QueryRowContext(ctx,
		`SELECT name FROM sqlite_schema WHERE type = 'table' AND name = 'reversals'`).Scan(&name)
	if err == nil {
		t.Fatal("embeddedSchema now carries a reversals table. TestBootPath_" +
			"ServerSchemaCarriesMigration0004 is therefore no longer a regression guard " +
			"against the old wiring -- it would pass either way. Delete this test and " +
			"rewrite that one's claim, or keep the schemas apart.")
	}
	if err != sql.ErrNoRows {
		t.Fatalf("probing embeddedSchema for a reversals table: %v", err)
	}
}

// --- helpers ---------------------------------------------------------------------

// buildLedgerd compiles cmd/ledgerd into dir and returns the binary path. The module
// root is the parent of this package's directory.
func buildLedgerd(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "ledgerd")
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("no go toolchain on PATH, cannot build the real server binary: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/ledgerd")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build cmd/ledgerd: %v\n%s", err, out)
	}
	return bin
}

// waitForSchema polls path until the named table appears, then returns an open handle to
// it. It fails fast if the server process exits first -- a crashed boot is a failure to
// report with its output, not something to wait out.
func waitForSchema(t *testing.T, path, table string, out *bytes.Buffer, proc *exec.Cmd) *sql.DB {
	t.Helper()
	deadline := time.Now().Add(bootTimeout)
	for {
		if _, err := os.Stat(path); err == nil {
			db, err := sql.Open("sqlite", path+dsn)
			if err == nil {
				var name string
				qerr := db.QueryRow(
					`SELECT name FROM sqlite_schema WHERE type='table' AND name=?`, table).Scan(&name)
				if qerr == nil {
					t.Cleanup(func() { _ = db.Close() })
					return db
				}
				_ = db.Close()
			}
		}
		if proc.ProcessState != nil && proc.ProcessState.Exited() {
			t.Fatalf("ledgerd exited before creating the schema:\n%s", out.String())
		}
		if time.Now().After(deadline) {
			t.Fatalf("the booted server never created table %q within %s. Migration 0004 is "+
				"not reaching the wired boot path -- this is the embeddedSchema drift that "+
				"shipped the collision-v2 500.\nserver output:\n%s", table, bootTimeout, out.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

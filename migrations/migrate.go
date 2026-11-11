// Package migrations holds the numbered, forward-only SQL schema for the ledger and
// a tiny applier that runs them in prefix order, idempotently (R14). The SQL files
// are the authoritative schema; internal/sqlite's embeddedSchema mirrors them so the
// adapter's own tests can stand alone.
//
// Idempotency is defence in depth: every DDL statement is CREATE ... IF NOT EXISTS,
// AND a schema_migrations ledger records applied versions so an already-applied file
// is skipped entirely. Re-running Apply against a current DB touches nothing (R14).
package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed *.sql
var files embed.FS

// ledgerDDL bootstraps the applier's own bookkeeping table. It is created by the
// applier, not by 0001, so the applier can record 0001 as applied.
const ledgerDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
  version    TEXT PRIMARY KEY, -- the NNNN prefix, e.g. "0001"
  name       TEXT NOT NULL,    -- full filename, for audit
  applied_at TEXT NOT NULL     -- RFC3339 UTC
)`

// Names returns the embedded migration filenames in prefix (apply) order.
func Names() ([]string, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // four-digit prefix: lexical sort == apply order
	return names, nil
}

// Apply runs every embedded migration not yet recorded, in prefix order. It is a
// no-op when the DB is already current (R14). It refuses a gap or a duplicate prefix.
func Apply(db *sql.DB) error {
	if _, err := db.Exec(ledgerDDL); err != nil {
		return fmt.Errorf("migrations: ledger table: %w", err)
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}
	names, err := Names()
	if err != nil {
		return err
	}

	var lastVer string
	for _, name := range names {
		ver := version(name)
		if lastVer != "" && ver <= lastVer {
			return fmt.Errorf("migrations: non-increasing or duplicate version %q after %q", ver, lastVer)
		}
		lastVer = ver
		if err := applyNamed(db, name, applied); err != nil {
			return err
		}
	}
	return nil
}

// ApplyOne applies a single named migration idempotently. It exists for tests that
// evolve a DB one migration at a time (bootstrap parity, R16); Apply is the real path.
func ApplyOne(db *sql.DB, name string) error {
	if _, err := db.Exec(ledgerDDL); err != nil {
		return fmt.Errorf("migrations: ledger table: %w", err)
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}
	return applyNamed(db, name, applied)
}

// applyNamed applies one migration file unless its version is already recorded. Leading
// PRAGMA statements run OUTSIDE the transaction (journal_mode=WAL silently fails to take
// effect inside one, 30-schema §1.3); the remaining DDL and the ledger row commit
// together, so a half-applied migration can never be recorded as done.
func applyNamed(db *sql.DB, name string, applied map[string]bool) error {
	ver := version(name)
	if applied[ver] {
		return nil // ledger says done -- the no-op path (R14)
	}
	body, err := files.ReadFile(name)
	if err != nil {
		return fmt.Errorf("migrations: read %s: %w", name, err)
	}
	pragmas, rest := splitLeadingPragmas(string(body))

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("migrations: %s pragma %q: %w", name, p, err)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("migrations: begin %s: %w", name, err)
	}
	if strings.TrimSpace(rest) != "" {
		// modernc.org/sqlite executes a multi-statement script in one Exec, so the
		// triggers (which carry inner semicolons) are not split and mangled.
		if _, err := tx.Exec(rest); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrations: %s failed: %w", name, err)
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations(version, name, applied_at) VALUES(?, ?, ?)`,
		ver, name, time.Now().UTC().Format(time.RFC3339)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("migrations: %s record: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrations: commit %s: %w", name, err)
	}
	applied[ver] = true
	return nil
}

func appliedVersions(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrations: read applied: %w", err)
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// version is the four-digit prefix of a migration filename ("0001_init.sql" -> "0001").
func version(name string) string {
	if len(name) < 4 {
		return name
	}
	return name[:4]
}

// splitLeadingPragmas separates any leading PRAGMA statements (which must run outside a
// transaction) from the rest of the migration body. The leading region may contain blank
// lines, comments, and single-line PRAGMA statements; it ends at the first real DDL line.
func splitLeadingPragmas(body string) (pragmas []string, rest string) {
	lines := strings.Split(body, "\n")
	i := 0
	for ; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "--"):
			continue // comment or blank: part of the leading region
		case strings.HasPrefix(strings.ToUpper(trimmed), "PRAGMA"):
			pragmas = append(pragmas, trimmed)
			continue
		}
		break // first real (non-pragma) statement: rest starts here
	}
	rest = strings.Join(lines[i:], "\n")
	return pragmas, rest
}

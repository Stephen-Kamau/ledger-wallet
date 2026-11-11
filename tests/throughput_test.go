package tests

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// baselineFile is the recorded artifact (30-schema §5). It is a baseline captured on one
// machine with one driver -- "write ~ X tps / read ~ Y rps at this commit" -- never a
// repo-wide performance guarantee.
const baselineFile = "throughput_baseline.txt"

// TestR19_ThroughputBaseline records write (transfer) and read (balance) throughput at
// concurrency 1/8/64 and asserts zero failed invariants under load (R19). Writes run from
// the AllowNegative system account so R3 never rejects and the number reflects the raw
// serialized-writer ceiling (§5), which is the honest thing to report. Counts are modest
// so the whole test runs in a few seconds.
func TestR19_ThroughputBaseline(t *testing.T) {
	l, db := newLedger(t)
	ctx := context.Background()
	sys := newSystemAccount(t, l)
	dst := newAccount(t, l, "alice")

	var rows []string
	rows = append(rows, "op|concurrency|n|wall_ms|ops_per_sec|infra_retries|failed_invariants")

	const writesPerRound = 1000
	round := 0
	for _, c := range []int{1, 8, 64} {
		round++
		r := round
		var writeIdx int64
		wall, infra := drive(c, writesPerRound, func(int) error {
			i := atomic.AddInt64(&writeIdx, 1)
			_, err := l.Transfer(ctx, core.TransferCmd{
				Authorizer: sysOwner, From: sys, To: dst,
				Amount: 1, Currency: usd,
				IdempotencyKey: fmt.Sprintf("w-%d-%d", r, i),
			})
			return err
		})
		failed := assertInvariants(t, db, l, sys, dst)
		rows = append(rows, row("write", c, writesPerRound, wall, infra, failed))
		t.Logf("write c=%d: %d ops in %v (%.0f ops/s) infra=%d", c, writesPerRound, wall, opsPerSec(writesPerRound, wall), infra)
	}

	// Read throughput: concurrent Balance reads against the hot account that absorbed every
	// write. WAL lets readers proceed without blocking the writer (§5), but balance is DERIVED
	// (R10: SUM over the account's postings, no stored field), so a hot account's read cost
	// grows O(postings) -- the honest baseline shows reads tracking, not dwarfing, writes here,
	// and the pure-Go modernc driver (~0.6ms/query) does not scale read latency down with cores.
	const reads = 1200
	for _, c := range []int{1, 8, 64} {
		wall, infra := drive(c, reads, func(int) error {
			_, err := l.Balance(ctx, dst)
			return err
		})
		failed := assertInvariants(t, db, l, sys, dst)
		rows = append(rows, row("read", c, reads, wall, infra, failed))
		t.Logf("read c=%d: %d ops in %v (%.0f ops/s) infra=%d", c, reads, wall, opsPerSec(reads, wall), infra)
	}

	// The release-gate assertion: whatever the throughput, correctness held.
	if failed := assertInvariants(t, db, l, sys, dst); failed != 0 {
		t.Fatalf("R19: %d failed invariants under load", failed)
	}

	if err := os.WriteFile(baselineFile, []byte(fmtRows(rows)), 0o644); err != nil {
		t.Fatalf("R19: write baseline: %v", err)
	}
	t.Logf("R19: baseline written to %s", baselineFile)
}

// drive runs n operations across `concurrency` goroutines, retrying only infra busy/locked
// errors. It fails nothing itself -- op errors other than infra are surfaced to the caller
// via panic-free path: here every op is expected to succeed, so a non-infra error aborts.
func drive(concurrency, n int, op func(i int) error) (wall time.Duration, infraRetries int64) {
	var infra int64
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for w := 0; w < concurrency; w++ {
		go func(w int) {
			defer wg.Done()
			for i := w; i < n; i += concurrency {
				for attempt := 0; attempt < 50; attempt++ {
					err := op(i)
					if err == nil {
						break
					}
					if isInfraBusy(err) {
						atomic.AddInt64(&infra, 1)
						continue
					}
					panic(fmt.Sprintf("drive op %d: unexpected error: %v", i, err))
				}
			}
		}(w)
	}
	wg.Wait()
	return time.Since(start), infra
}

func opsPerSec(n int, wall time.Duration) float64 {
	if wall <= 0 {
		return 0
	}
	return float64(n) / wall.Seconds()
}

func row(op string, c, n int, wall time.Duration, infra int64, failed int) string {
	return fmt.Sprintf("%s|%d|%d|%d|%.0f|%d|%d",
		op, c, n, wall.Milliseconds(), opsPerSec(n, wall), infra, failed)
}

func fmtRows(rows []string) string {
	out := ""
	for _, r := range rows {
		out += r + "\n"
	}
	return out
}

package main

import (
	"context"
	"flag"
	"log"
)

// main is deliberately tiny: it wires the ledger and reports readiness. The HTTP
// surface is the api stream's (Dakari) to mount onto the core.Ledger that New
// returns. Keeping transport out of here is what lets api own the boundary.
func main() {
	dbPath := flag.String("db", "ledger.db", "path to the SQLite database file")
	flag.Parse()

	ctx := context.Background()
	ledger, db, err := New(ctx, *dbPath)
	if err != nil {
		log.Fatalf("ledgerd: wiring failed: %v", err)
	}
	defer db.Close()
	_ = ledger

	log.Printf("ledgerd: ledger ready on %s (api layer mounts the HTTP surface)", *dbPath)
}

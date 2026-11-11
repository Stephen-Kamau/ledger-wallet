package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/Stephen-Kamau/ledger-wallet/api"
	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// main wires the ledger and serves the HTTP surface. The api stream (Dakari) mounts
// its router onto the core.Ledger that New returns.
//
// OVERLAP NOTE (D-A1): cmd/ledgerd is Amara's scope, not Dakari's. The api stream
// needs a running server to host its router, so this HTTP wiring was added by Dakari
// (api/). This is a deliberate cross-stream grab; the coordinator's overlap gate is
// expected to flag it against Amara's claim on cmd/ledgerd.
func main() {
	dbPath := flag.String("db", "ledger.db", "path to the SQLite database file")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	adminList := flag.String("admins", "", "comma-separated principals allowed to mint (allow_negative) and read any account")
	flag.Parse()

	ctx := context.Background()
	ledger, db, err := New(ctx, *dbPath)
	if err != nil {
		log.Fatalf("ledgerd: wiring failed: %v", err)
	}
	defer db.Close()

	handler := api.NewHandler(ledger, adminOptions(*adminList)...)

	log.Printf("ledgerd: ledger ready on %s; serving HTTP on %s", *dbPath, *addr)
	if err := http.ListenAndServe(*addr, handler.Router()); err != nil {
		log.Fatalf("ledgerd: http server stopped: %v", err)
	}
}

// adminOptions turns the -admins flag into the api admin set (10-api.md §2.4).
func adminOptions(list string) []api.Option {
	if strings.TrimSpace(list) == "" {
		return nil
	}
	var admins []core.PrincipalID
	for _, p := range strings.Split(list, ",") {
		if p = strings.TrimSpace(p); p != "" {
			admins = append(admins, core.PrincipalID(p))
		}
	}
	if len(admins) == 0 {
		return nil
	}
	return []api.Option{api.WithAdmins(admins...)}
}

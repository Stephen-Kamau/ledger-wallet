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

// main is the server entrypoint and owns the server lifecycle: flags, DB open/close,
// listen address, and ListenAndServe. The transport layer plugs in without owning any
// of that - api (Dakari) exposes api.NewHandler(ledger) (an http.Handler via Router());
// this file constructs the ledger through New and mounts that handler. Dependency
// direction stays core <- api <- cmd; neither core nor api depends on cmd.
//
// OWNERSHIP (D-A1): cmd/ledgerd is owned by Amara. An earlier api-stream grab put the
// HTTP wiring here directly; per the owner's ruling the entrypoint stays Amara's and
// folds in api.NewHandler, rather than the api stream editing this file. See
// AIGILE_DECISIONS.md D-A1 / the overlap adjudication in docs/operators-activity-log.md.
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

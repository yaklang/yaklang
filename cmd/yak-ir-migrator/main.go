// Command yak-ir-migrator is the standalone DDL authority for the SSA IR DB.
//
// It is the ONLY component in the platform that runs DDL on the shared IR DB.
// scannode never runs DDL (it calls irschema.Check, a read-only version gate,
// per task). legion-control shells out to this binary via `legion-control
// ir-migrate` and then records the resulting version in business PG; it never
// imports the yaklang engine tree.
//
// The binary imports only common/yak/ssa/ssadb/irschema (the leaf schema
// package) — no SSA runtime, no scannode, no yak engine. Build it without
// build tags:
//
//	go build -o yak-ir-migrator ./cmd/yak-ir-migrator
//
// Usage:
//
//	yak-ir-migrator --dsn <postgres-ir-db-dsn> [--to-version N] [--force-adopt]
//
// Exit codes:
//
//	0 — migrated (or already at version); prints {"version": N} on stdout
//	2 — adoption drift (existing IR DB schema does not match baseline);
//	    prints a JSON drift report on stdout for legion-control to surface
//	3 — already at the requested version (no-op success)
//	1 — other error (DSN unreadable, lock unavailable, etc.)
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb/irschema"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		// Adoption drift → exit 2 (structured report), other errors → exit 1.
		if errors.Is(err, irschema.ErrIRSchemaAdoptDrift) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if errors.Is(err, irschema.ErrIRSchemaAlreadyAtVersion) {
			// Already at version is a no-op success for idempotent deploys.
			fmt.Println(`{"status":"already_at_version"}`)
			os.Exit(3)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("yak-ir-migrator", flag.ContinueOnError)
	dsn := fs.String("dsn", "", "Postgres DSN of the SSA IR DB (e.g. postgres://user:pass@host:5436/db?sslmode=disable)")
	toVersion := fs.Int64("to-version", 0, "target schema version (0 = up to the max embedded version)")
	forceAdopt := fs.Bool("force-adopt", false, "DESTRUCTIVE: stamp v1 on a pre-governance IR DB even if its schema drifts from the baseline. Use only after manual review.")
	verbose := fs.Bool("verbose", false, "enable debug logging")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" {
		return fmt.Errorf("--dsn is required")
	}
	if !*verbose {
		log.SetLevel(2) // info
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// gorm v1 open. We never call AutoMigrate here (that path is gated by
	// ENV_SSA_DB_SKIP_MIGRATE in consts; but to be belt-and-suspenders we
	// open the DB directly via gorm without going through consts, so the
	// migrator has zero dependency on the consts init path).
	db, err := gorm.Open("postgres", *dsn)
	if err != nil {
		return fmt.Errorf("open IR DB: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.DB().Ping(); err != nil {
		return fmt.Errorf("ping IR DB: %w", err)
	}

	version, err := irschema.Migrate(ctx, db, irschema.MigrateOptions{
		ToVersion:  *toVersion,
		ForceAdopt: *forceAdopt,
	})
	if err != nil {
		return err
	}

	// Print a machine-readable success line for legion-control to parse.
	out := map[string]any{"status": "migrated", "version": version}
	raw, _ := json.Marshal(out)
	fmt.Println(string(raw))
	return nil
}

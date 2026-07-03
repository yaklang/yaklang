package scannode

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb/irschema"
)

// TestCheckIRSchemaVersion_NoDSN: when no SSA_DATABASE_RAW is injected
// (legacy/local dev mode), the gate is a no-op — scannode falls back to the
// consts path which AutoMigrates a local SQLite/temp DB. This keeps the
// single-node yakit/CLI workflow unchanged.
func TestCheckIRSchemaVersion_NoDSN(t *testing.T) {
	s := &ScanNode{}
	ctx := context.Background()
	if err := s.checkIRSchemaVersion(ctx, nil, 0, nil); err != nil {
		t.Fatalf("no-DSN path must be a no-op (nil error), got %v", err)
	}
	if err := s.checkIRSchemaVersion(ctx, []string{}, 0, nil); err != nil {
		t.Fatalf("empty-env path must be a no-op, got %v", err)
	}
}

// TestCheckIRSchemaVersion_SchedulerTooOld: when the scheduler's injected
// expected version is below this binary's MinSupported, the gate rejects
// WITHOUT opening a DB connection (cheap short-circuit).
func TestCheckIRSchemaVersion_SchedulerTooOld(t *testing.T) {
	if irschema.MinSupportedIRSchemaVersion <= 0 {
		t.Skip("MinSupported is 0; scheduler-too-old short-circuit only triggers when MinSupported>0 (future binary)")
	}
	s := &ScanNode{}
	ctx := context.Background()
	// Unreachable DSN; the short-circuit must trigger before connect.
	env := []string{"SSA_DATABASE_RAW=postgres://x:y@127.0.0.1:1/db?sslmode=disable"}
	err := s.checkIRSchemaVersion(ctx, env, irschema.MinSupportedIRSchemaVersion-1, nil)
	if err == nil {
		t.Fatal("expected ErrIRSchemaIncompatible for scheduler-too-old, got nil")
	}
	if !errors.Is(err, ErrIRSchemaIncompatible) {
		t.Fatalf("expected ErrIRSchemaIncompatible, got %v", err)
	}
}

// TestIRDSNFromEnv: irDSNFromEnv extracts the SSA_DATABASE_RAW DSN from a
// KEY=VALUE env slice.
func TestIRDSNFromEnv_GatePkg(t *testing.T) {
	got := irDSNFromEnv([]string{"SSA_DATABASE_RAW=postgres://x@y/z", "SSA_DB_SKIP_MIGRATE=1"})
	if got != "postgres://x@y/z" {
		t.Fatalf("got %q", got)
	}
	if got := irDSNFromEnv(nil); got != "" {
		t.Fatalf("expected empty for nil, got %q", got)
	}
}

// TestCheckIRSchemaVersion_RealDB is an integration test that exercises the
// full gate against a live Postgres 16: a freshly-migrated DB (Compatible)
// is allowed; the gate reports success. Env-gated like irschema's drift test.
//
// It does NOT exercise the incompatible path here — that path is unit-covered
// by TestCheckIRSchemaVersion_SchedulerTooOld and the irschema package's own
// Check tests (TestCheck_TooNew / TestCheck_UnmanagedDB). The point of this
// test is end-to-end: the scannode gate opens the DSN, calls irschema.Check,
// and returns nil on a compatible DB.
func TestCheckIRSchemaVersion_RealDB(t *testing.T) {
	rootDSN := resolveRootDSNForGateTest()
	if rootDSN == "" || os.Getenv("TEST_POSTGRES") == "" {
		t.Skip("set TEST_POSTGRES=1 and IR_DB_TEST_DSN (or PALM_POSTGRES_*) to run the scannode gate integration test")
	}
	// Reuse a freshly-migrated DB. We provision one by applying the baseline
	// SQL + stamping v1, mirroring what yak-ir-migrator does.
	ctx := context.Background()
	dbName := "ir_gate_real"
	migratedDSN := provisionMigratedDBForGateTest(t, rootDSN, dbName)

	s := &ScanNode{}
	env := []string{"SSA_DATABASE_RAW=" + migratedDSN}
	if err := s.checkIRSchemaVersion(ctx, env, 0, nil); err != nil {
		t.Fatalf("migrated DB should be compatible (gate returns nil), got %v", err)
	}
}
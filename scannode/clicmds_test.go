package scannode

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
)

func TestRunDistYakFileUsesExecutionContext(t *testing.T) {
	script, err := os.CreateTemp(t.TempDir(), "distyak-context-*.yak")
	if err != nil {
		t.Fatalf("create script: %v", err)
	}
	if _, err := script.WriteString(`time.sleep(0.5)`); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := script.Close(); err != nil {
		t.Fatalf("close script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = runDistYakFile(ctx, script.Name(), "test-runtime")
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("distyak context cancellation took too long: %v", elapsed)
	}
}

// TestDistYakCommandConsumesSSADatabaseEnv verifies that the distyak entry
// point reads SSA_DATABASE_RAW before script execution so the scheduler can
// redirect compile/scan IR to a shared Postgres IR DB. This pins the bug fix
// where the env var was silently ignored and IR fell back to default SQLite.
func TestDistYakCommandConsumesSSADatabaseEnv(t *testing.T) {
	original := consts.GetSSADatabaseInfoFromEnv()
	_, originalRaw := consts.GetSSADataBaseInfo()
	t.Cleanup(func() {
		t.Setenv(consts.ENV_SSA_DATABASE_RAW, original)
		consts.SetSSADatabaseInfo(originalRaw)
		consts.SetSSADatabaseSkipMigrate(false)
	})

	const fakeDSN = "postgres://testuser:testpass@127.0.0.1:5436/ssa_ir_test?sslmode=disable"
	t.Setenv(consts.ENV_SSA_DATABASE_RAW, fakeDSN)

	// Exercise the same env-var consumption path the CLI Action runs.
	applySSADatabaseFromEnv()

	// After Action ran, the global SSA DB info must reflect the env DSN.
	_, raw := consts.GetSSADataBaseInfo()
	if raw != fakeDSN {
		t.Fatalf("expected SSA DB raw to be %q, got %q (env var was not consumed)", fakeDSN, raw)
	}
	if got := os.Getenv(consts.ENV_SSA_DATABASE_RAW); got != "" {
		t.Fatalf("SSA DSN remained visible to dispatched script: %q", got)
	}
}

func TestApplySSADatabaseFromEnvConsumesSkipMigrate(t *testing.T) {
	_, originalRaw := consts.GetSSADataBaseInfo()
	t.Cleanup(func() { consts.SetSSADatabaseInfo(originalRaw) })
	t.Setenv(consts.ENV_SSA_DATABASE_RAW, "postgres://scan@db.example/company_1")
	t.Setenv(consts.ENV_SSA_DB_SKIP_MIGRATE, "1")
	t.Cleanup(func() { consts.SetSSADatabaseSkipMigrate(false) })

	applySSADatabaseFromEnv()
	if !consts.SSADatabaseSkipMigrate() {
		t.Fatal("skip-migrate policy was not retained after consuming the environment")
	}
	if os.Getenv(consts.ENV_SSA_DATABASE_RAW) != "" || os.Getenv(consts.ENV_SSA_DB_SKIP_MIGRATE) != "" {
		t.Fatal("SSA database policy remained visible to dispatched script")
	}
}

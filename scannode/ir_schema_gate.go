package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb/irschema"
)

// ErrIRSchemaIncompatible is returned by executeScriptTask when the IR DB
// schema version is not in the binary's compatibility window. The task is
// non-retryable on this node until the node is upgraded (DB newer than the
// binary) or the migrator catches up (DB older than the binary).
//
// Fail-fast, not fail-soft: we refuse to run the script rather than
// silently falling back to a temp SQLite DB, because silent IR-reuse
// loss is very hard for operators to notice. The scheduler sees the
// terminal job failure and surfaces it as a first-class alert.
var ErrIRSchemaIncompatible = errors.New("scannode: SSA IR DB schema version incompatible with this node")

// irSchemaIncompatiblePayload is the structured event emitted via the
// ScanResult_IRSchemaIncompatible feedback channel. It carries the version
// triplet so the scheduler / console can show exactly what diverged.
type irSchemaIncompatiblePayload struct {
	CurrentVersion  int64  `json:"current_version"`
	ExpectedVersion int64  `json:"expected_version"`
	MinSupported    int64  `json:"min_supported"`
	MissingVersions []int64 `json:"missing_versions,omitempty"`
	Node            string `json:"node,omitempty"`
}

// checkIRSchemaVersion is the per-task version gate (RFC §5). scannode is
// long-running, so a per-process check at startup would miss mid-life
// migrations; therefore each compile/scan task checks the version immediately
// before executing the yak script.
//
// ssaDBEnv carries the scheduler-injected SSA_DATABASE_RAW (a DML-only DSN).
// We open a short-lived pooled connection, run irschema.Check (read-only —
// safe under a DML-only role), then close it. The actual script run opens
// its own connection from the same DSN via consts.
//
// On incompatibility we publish a terminal job failure (so the scheduler
// stops dispatching to this node version) and return ErrIRSchemaIncompatible.
// On success or when no DSN is configured (legacy/local mode), we proceed.
func (s *ScanNode) checkIRSchemaVersion(
	ctx context.Context,
	ssaDBEnv []string,
	expectedFromScheduler int64,
	reporter *ScannerAgentReporter,
) error {
	dsn := irDSNFromEnv(ssaDBEnv)
	if dsn == "" {
		// No shared IR DB configured (local dev, legacy scannode). The consts
		// path will AutoMigrate a local SQLite/temp DB as before. Skip the
		// gate — there is nothing to be incompatible with.
		return nil
	}

	// Cheap short-circuit: if the scheduler's expected version is below this
	// binary's MinSupported, reject without touching the DB. This catches
	// stale scheduler config before opening a connection.
	if expectedFromScheduler > 0 && expectedFromScheduler < irschema.MinSupportedIRSchemaVersion {
		return s.failIncompatible(ctx, reporter, irschema.CheckResult{
			CurrentVersion:  expectedFromScheduler,
			ExpectedVersion: irschema.CurrentIRSchemaVersion,
			MinSupported:    irschema.MinSupportedIRSchemaVersion,
		}, "scheduler-expected version below this node's MinSupported")
	}

	db, err := gorm.Open("postgres", dsn)
	if err != nil {
		// Cannot even open the IR DB. Do NOT mask this as "incompatible" —
		// it is a connectivity/credential problem the scheduler should retry
		// elsewhere, not a version mismatch. Surface as a normal exec error.
		return fmt.Errorf("open IR DB for version check: %w", err)
	}
	defer func() { _ = db.Close() }()

	res, err := irschema.Check(ctx, db)
	if err != nil {
		// Read failure (e.g. ir_schema_migrations unreadable under a DML-only
		// role that lacks SELECT on it). Treat as incompatible rather than
		// retryable: the grant scripts must grant SELECT on
		// ir_schema_migrations to ir_dml_user.
		return s.failIncompatible(ctx, reporter, irschema.CheckResult{
			ExpectedVersion: irschema.CurrentIRSchemaVersion,
			MinSupported:    irschema.MinSupportedIRSchemaVersion,
		}, fmt.Sprintf("version check read failed: %v", err))
	}
	if res.Compatible {
		log.Infof("scannode: IR schema check ok (DB at v%d, binary expects v%d, min v%d)",
			res.CurrentVersion, res.ExpectedVersion, res.MinSupported)
		return nil
	}
	return s.failIncompatible(ctx, reporter, res, reasonFor(res))
}

// failIncompatible emits the structured ScanResult_IRSchemaIncompatible
// feedback (so the scheduler/console can surface it) and returns
// ErrIRSchemaIncompatible (so executeScriptTask aborts before invoking the
// script).
func (s *ScanNode) failIncompatible(
	_ context.Context,
	reporter *ScannerAgentReporter,
	res irschema.CheckResult,
	reason string,
) error {
	log.Warnf("scannode: IR schema incompatible — %s; DB v%d, binary expects v%d (min v%d), missing=%v",
		reason, res.CurrentVersion, res.ExpectedVersion, res.MinSupported, res.MissingMigrations)
	payload := irSchemaIncompatiblePayload{
		CurrentVersion:  res.CurrentVersion,
		ExpectedVersion: res.ExpectedVersion,
		MinSupported:    res.MinSupported,
		MissingVersions: res.MissingMigrations,
	}
	raw, _ := json.Marshal(payload)
	// Best-effort: publish a terminal job failure so the scheduler sees the
	// non-retryable reason. Errors publishing are logged but do not mask the
	// incompatibility — the task is rejected either way.
	if reporter != nil {
		if pub, ref, ok, _ := reporter.legionPublisher(); ok && pub != nil && ref != nil {
			detail := map[string]string{
				"reason":          reason,
				"current_version": fmt.Sprintf("%d", res.CurrentVersion),
				"expected_version": fmt.Sprintf("%d", res.ExpectedVersion),
				"min_supported":   fmt.Sprintf("%d", res.MinSupported),
				"payload_json":    string(raw),
			}
			_ = pub.PublishFailed(reporter.agent.node.GetRootContext(), *ref,
				"ir_schema_incompatible",
				fmt.Sprintf("SSA IR DB schema version incompatible: %s", reason),
				detail,
			)
		}
	}
	return fmt.Errorf("%w: %s (DB v%d, binary expects v%d)", ErrIRSchemaIncompatible, reason, res.CurrentVersion, res.ExpectedVersion)
}

func reasonFor(res irschema.CheckResult) string {
	switch {
	case res.CurrentVersion > res.ExpectedVersion:
		return "DB schema is newer than this node understands (rolling-upgrade: drain this node)"
	case res.CurrentVersion < res.MinSupported:
		return fmt.Sprintf("DB schema is too old (v%d < min v%d); run legion-control ir-migrate", res.CurrentVersion, res.MinSupported)
	default:
		return "schema version mismatch"
	}
}

// irDSNFromEnv extracts the SSA_DATABASE_RAW value from a "KEY=VALUE" env
// slice produced by extractSSADatabaseEnv. Returns "" if absent.
func irDSNFromEnv(env []string) string {
	const prefix = "SSA_DATABASE_RAW="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return strings.TrimPrefix(kv, prefix)
		}
	}
	return ""
}
package scannode

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
)

const scannodeInternalParamPrefix = "_scannode_"

// Hidden param keys consumed by scannode to set the shared SSA IR DB DSN
// and skip AutoMigrate on read-only (scan) nodes. The legion scheduler
// injects these into the script input JSON; scannode extracts them and
// forwards as env vars to the distyak child process.
const (
	scannodeSSADatabaseRawParamKey           = "_scannode_ssa_database_raw"
	scannodeSSASkipMigrateParamKey           = "_scannode_ssa_skip_migrate"
	scannodeSSAExpectedIRSchemaVersionParamKey = "_scannode_ssa_expected_ir_schema_version"
)

// extractSSADatabaseEnv reads the shared SSA IR DB DSN from the scheduler-injected
// hidden params and returns "KEY=VALUE" env var entries for the distyak child
// process.
//
// As of the SSA IR DB migration governance design (see
// docs/design-docs/ssa-ir-db-migration-governance.md), scannode NEVER runs DDL
// on the IR DB. SSA_DB_SKIP_MIGRATE=1 is set unconditionally whenever a DSN is
// injected — for both compile and scan workloads. The migrator
// (cmd/yak-ir-migrator) is the sole DDL authority; scannode holds only a
// DML-only role (ir_dml_user) and a per-task irschema.Check version gate
// (see script_execution.go).
//
// Returns nil if no DSN was injected (legacy/unconfigured mode).
func extractSSADatabaseEnv(params map[string]interface{}) []string {
	if len(params) == 0 {
		return nil
	}
	dsn := strings.TrimSpace(toString(params[scannodeSSADatabaseRawParamKey]))
	if dsn == "" {
		return nil
	}
	// Unconditional skip_migrate — supersedes the prior compile=skip_migrate:false
	// transitional state (Track B). scannode never runs DDL on the shared IR DB.
	env := []string{
		fmt.Sprintf("%s=%s", consts.ENV_SSA_DATABASE_RAW, dsn),
		fmt.Sprintf("%s=1", consts.ENV_SSA_DB_SKIP_MIGRATE),
	}
	return env
}

// extractExpectedIRSchemaVersion reads the scheduler-injected expected IR
// schema version. scannode uses it as a cheap short-circuit: if the scheduler
// is still configured for an older schema than this binary expects, the task
// is rejected before even opening the IR DB. Returns 0 if not injected
// (legacy mode — the per-task irschema.Check gate still runs against the DB).
func extractExpectedIRSchemaVersion(params map[string]interface{}) int64 {
	if len(params) == 0 {
		return 0
	}
	return toInt64(params[scannodeSSAExpectedIRSchemaVersionParamKey])
}

func extractSSAArtifactUploadConfig(params map[string]interface{}) *SSAArtifactUploadConfig {
	if len(params) == 0 {
		return nil
	}
	cfg := &SSAArtifactUploadConfig{
		ObjectKey: strings.TrimSpace(toString(params["_scannode_ssa_object_key"])),
		Codec:     strings.TrimSpace(toString(params["_scannode_ssa_codec"])),
		Endpoint:  strings.TrimSpace(toString(params["_scannode_ssa_endpoint"])),
		Bucket:    strings.TrimSpace(toString(params["_scannode_ssa_bucket"])),
		Region:    strings.TrimSpace(toString(params["_scannode_ssa_region"])),
		UseSSL:    toBool(params["_scannode_ssa_use_ssl"]),

		STSAccessKey:    strings.TrimSpace(toString(params["_scannode_ssa_sts_access_key"])),
		STSSecretKey:    strings.TrimSpace(toString(params["_scannode_ssa_sts_secret_key"])),
		STSSessionToken: strings.TrimSpace(toString(params["_scannode_ssa_sts_session_token"])),
		STSExpiresAt:    toInt64(params["_scannode_ssa_sts_expires_at"]),
	}
	if cfg.Codec == "" {
		cfg.Codec = "zstd"
	}
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.ObjectKey == "" {
		return nil
	}
	return cfg
}

func (cfg *SSAArtifactUploadConfig) NeedSTSRefresh(renewBeforeSec int64) bool {
	if cfg == nil {
		return true
	}
	if strings.TrimSpace(cfg.STSAccessKey) == "" || strings.TrimSpace(cfg.STSSecretKey) == "" {
		return true
	}
	if renewBeforeSec <= 0 {
		renewBeforeSec = 600
	}
	if cfg.STSExpiresAt <= 0 {
		return false
	}
	return time.Now().Unix() >= cfg.STSExpiresAt-renewBeforeSec
}

func toString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func toBool(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		t = strings.TrimSpace(strings.ToLower(t))
		return t == "1" || t == "true" || t == "yes" || t == "on"
	default:
		return false
	}
}

func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case string:
		var n int64
		for _, ch := range strings.TrimSpace(t) {
			if ch < '0' || ch > '9' {
				return 0
			}
			n = n*10 + int64(ch-'0')
		}
		return n
	default:
		return 0
	}
}

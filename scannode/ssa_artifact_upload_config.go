package scannode

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

const scannodeInternalParamPrefix = "_scannode_"

// Hidden param keys consumed by scannode to set the shared SSA IR DB DSN
// and skip AutoMigrate on read-only (scan) nodes. The legion scheduler
// injects these into the script input JSON; scannode extracts them and
// forwards as env vars to the distyak child process.
const (
	scannodeSSADatabaseRawParamKey     = "_scannode_ssa_database_raw"
	scannodeSSASkipMigrateParamKey     = "_scannode_ssa_skip_migrate"
	scannodeSSADatabaseBackendParamKey = "_scannode_ssa_database_backend"
	scannodeSSASqliteKeyParamKey       = "_scannode_ssa_sqlite_key"
)

const ssaIRSQLiteDirName = "ssa-ir-sqlite"

// extractSSADatabaseEnv reads the shared SSA IR DB DSN and skip_migrate flag
// from the scheduler-injected hidden params and returns them as "KEY=VALUE"
// env var entries suitable for appending to a child process cmd.Env.
// Returns nil if no DSN was injected (legacy/unconfigured mode).
func extractSSADatabaseEnv(params map[string]interface{}) []string {
	if len(params) == 0 {
		return nil
	}
	dsn := strings.TrimSpace(toString(params[scannodeSSADatabaseRawParamKey]))
	if dsn == "" {
		return nil
	}
	env := []string{
		fmt.Sprintf("%s=%s", consts.ENV_SSA_DATABASE_RAW, dsn),
	}
	if toBool(params[scannodeSSASkipMigrateParamKey]) {
		env = append(env, fmt.Sprintf("%s=1", consts.ENV_SSA_DB_SKIP_MIGRATE))
	}
	return env
}

func isSQLiteSSABackend(params map[string]interface{}) bool {
	b := strings.ToLower(strings.TrimSpace(toString(params[scannodeSSADatabaseBackendParamKey])))
	return b == "sqlite" || b == "sqlite3"
}

func scanNodeIRBaseDir(s *ScanNode) string {
	if s != nil && s.node != nil {
		if base := strings.TrimSpace(s.node.BaseDir()); base != "" {
			return base
		}
	}
	return filepath.Join(os.TempDir(), "legion-node")
}

// resolveSSADatabaseEnv chooses the child SSA_DATABASE_RAW env.
// Postgres (default): hidden DSN param, same as extractSSADatabaseEnv.
// SQLite: live file at {nodeBase}/ssa-ir-sqlite/{key}/ssadb.db so compile and
// scan jobs that share a key reuse the same IR database. debugDir is not the
// live path; finalize copies the file into the debug zip.
func resolveSSADatabaseEnv(s *ScanNode, params map[string]interface{}, debugDir, runtimeID string) (env []string, sqlitePath string) {
	return resolveSSADatabaseEnvWithBase(scanNodeIRBaseDir(s), params, debugDir, runtimeID)
}

func resolveSSADatabaseEnvWithBase(nodeBase string, params map[string]interface{}, debugDir, runtimeID string) (env []string, sqlitePath string) {
	if !isSQLiteSSABackend(params) {
		return extractSSADatabaseEnv(params), ""
	}
	key := strings.TrimSpace(toString(params[scannodeSSASqliteKeyParamKey]))
	if key == "" {
		key = strings.TrimSpace(runtimeID)
	}
	if key == "" {
		log.Warnf("[ssadb] sqlite backend requested but sqlite key and runtime id are empty; falling back to DSN env")
		return extractSSADatabaseEnv(params), ""
	}
	key = sanitizeLogName(key)
	nodeBase = strings.TrimSpace(nodeBase)
	if nodeBase == "" {
		nodeBase = filepath.Join(os.TempDir(), "legion-node")
	}
	sqlitePath = filepath.Join(nodeBase, ssaIRSQLiteDirName, key, "ssadb.db")
	if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o755); err != nil {
		log.Warnf("[ssadb] mkdir sqlite IR dir failed: %v", err)
		return extractSSADatabaseEnv(params), ""
	}
	log.Infof("[ssadb] using sqlite IR database: path=%s", sqlitePath)
	if debugDir != "" {
		log.Infof("[ssadb] debug zip will include a copy of sqlite IR from %s", sqlitePath)
	}
	env = []string{
		fmt.Sprintf("%s=%s", consts.ENV_SSA_DATABASE_RAW, sqlitePath),
	}
	if toBool(params[scannodeSSASkipMigrateParamKey]) {
		env = append(env, fmt.Sprintf("%s=1", consts.ENV_SSA_DB_SKIP_MIGRATE))
	}
	return env, sqlitePath
}

func extractSSAArtifactUploadConfig(params map[string]interface{}) *SSAArtifactUploadConfig {
	if len(params) == 0 {
		return nil
	}
	cfg := &SSAArtifactUploadConfig{
		ObjectKey:        strings.TrimSpace(toString(params["_scannode_ssa_object_key"])),
		Codec:            strings.TrimSpace(toString(params["_scannode_ssa_codec"])),
		Endpoint:         strings.TrimSpace(toString(params["_scannode_ssa_endpoint"])),
		Bucket:           strings.TrimSpace(toString(params["_scannode_ssa_bucket"])),
		Region:           strings.TrimSpace(toString(params["_scannode_ssa_region"])),
		UseSSL:           toBool(params["_scannode_ssa_use_ssl"]),
		TLSVerify:        toBool(params["_scannode_ssa_tls_verify"]),
		TLSCAFile:        strings.TrimSpace(toString(params["_scannode_ssa_tls_ca_file"])),
		AllowHTTP:        toBool(params["_scannode_ssa_allow_http"]),
		VirtualHostStyle: toBool(params["_scannode_ssa_virtual_host_style"]),

		STSAccessKey:    newSecretValue(toString(params["_scannode_ssa_sts_access_key"])),
		STSSecretKey:    newSecretValue(toString(params["_scannode_ssa_sts_secret_key"])),
		STSSessionToken: newSecretValue(toString(params["_scannode_ssa_sts_session_token"])),
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
	if cfg.STSAccessKey.raw() == "" || cfg.STSSecretKey.raw() == "" {
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

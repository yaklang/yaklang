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
	scannodeSSADatabaseRawParamKey = "_scannode_ssa_database_raw"
	scannodeSSASkipMigrateParamKey = "_scannode_ssa_skip_migrate"
)

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

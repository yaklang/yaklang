package scannode

import (
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
)

func TestExtractSSAArtifactUploadConfigSTS(t *testing.T) {
	cfg := extractSSAArtifactUploadConfig(map[string]interface{}{
		"_scannode_ssa_object_key":        "ssa/tasks/t1/ssa_result_parts.ndjson.zst",
		"_scannode_ssa_codec":             "zstd",
		"_scannode_ssa_endpoint":          "127.0.0.1:9000",
		"_scannode_ssa_bucket":            "irify-ssa",
		"_scannode_ssa_region":            "us-east-1",
		"_scannode_ssa_use_ssl":           false,
		"_scannode_ssa_sts_access_key":    "AKIA_TEMP",
		"_scannode_ssa_sts_secret_key":    "SECRET_TEMP",
		"_scannode_ssa_sts_session_token": "TOKEN_TEMP",
		"_scannode_ssa_sts_expires_at":    int64(1893456000),
	})
	if cfg == nil {
		t.Fatal("expected non-nil cfg")
	}
	if cfg.Endpoint != "127.0.0.1:9000" || cfg.Bucket != "irify-ssa" {
		t.Fatalf("unexpected endpoint/bucket: %+v", cfg)
	}
	if cfg.STSAccessKey == "" || cfg.STSSecretKey == "" {
		t.Fatalf("sts creds should be parsed: %+v", cfg)
	}
}

func TestExtractSSAArtifactUploadConfigNoSTS(t *testing.T) {
	cfg := extractSSAArtifactUploadConfig(map[string]interface{}{
		"_scannode_ssa_object_key": "ssa/tasks/t1/ssa_result_parts.ndjson.zst",
		"_scannode_ssa_codec":      "zstd",
		"_scannode_ssa_endpoint":   "127.0.0.1:9000",
		"_scannode_ssa_bucket":     "irify-ssa",
		"_scannode_ssa_region":     "us-east-1",
		"_scannode_ssa_use_ssl":    false,
	})
	if cfg == nil {
		t.Fatal("expected non-nil cfg without sts")
	}
	if !cfg.NeedSTSRefresh(600) {
		t.Fatal("expected refresh required when sts creds missing")
	}
}

func TestSSAArtifactUploadConfigNeedSTSRefresh(t *testing.T) {
	cfg := &SSAArtifactUploadConfig{
		STSAccessKey: "ak",
		STSSecretKey: "sk",
		STSExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
	}
	if !cfg.NeedSTSRefresh(600) {
		t.Fatal("expected refresh when token expires within renew window")
	}
	cfg.STSExpiresAt = time.Now().Add(30 * time.Minute).Unix()
	if cfg.NeedSTSRefresh(600) {
		t.Fatal("expected no refresh when token is still valid")
	}
}

func TestExtractSSADatabaseEnv(t *testing.T) {
	t.Run("returns DSN env when database_raw present", func(t *testing.T) {
		const dsn = "postgres://legion:legion@127.0.0.1:5436/ssa_ir?sslmode=disable"
		env := extractSSADatabaseEnv(map[string]interface{}{
			scannodeSSADatabaseRawParamKey: dsn,
			scannodeSSASkipMigrateParamKey: true,
		})
		if len(env) < 1 {
			t.Fatalf("expected at least 1 env entry, got %d", len(env))
		}
		if !strings.Contains(env[0], consts.ENV_SSA_DATABASE_RAW+"="+dsn) {
			t.Fatalf("expected SSA_DATABASE_RAW env, got %v", env)
		}
		var foundSkip bool
		for _, e := range env {
			if strings.Contains(e, consts.ENV_SSA_DB_SKIP_MIGRATE+"=1") {
				foundSkip = true
				break
			}
		}
		if !foundSkip {
			t.Fatalf("expected SSA_DB_SKIP_MIGRATE=1 in env, got %v", env)
		}
	})

	t.Run("returns nil when database_raw absent", func(t *testing.T) {
		env := extractSSADatabaseEnv(map[string]interface{}{
			"_scannode_ssa_object_key": "ssa/tasks/t1/result.ndjson.zst",
		})
		if env != nil {
			t.Fatalf("expected nil env when no DSN, got %v", env)
		}
	})

	t.Run("returns nil for empty params", func(t *testing.T) {
		env := extractSSADatabaseEnv(nil)
		if env != nil {
			t.Fatalf("expected nil for empty params, got %v", env)
		}
	})

	t.Run("omits skip_migrate when false", func(t *testing.T) {
		env := extractSSADatabaseEnv(map[string]interface{}{
			scannodeSSADatabaseRawParamKey: "postgres://x@y/db",
			scannodeSSASkipMigrateParamKey: false,
		})
		for _, e := range env {
			if strings.Contains(e, consts.ENV_SSA_DB_SKIP_MIGRATE) {
				t.Fatalf("did not expect skip_migrate env, got %v", env)
			}
		}
	})
}

package scannode

import (
	"os"
	"path/filepath"
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
	if !cfg.AllowHTTP {
		t.Fatal("legacy scheme-less HTTP endpoint should preserve the Legion scheduler contract")
	}
	if _, err := validateSSAUploadConfig(cfg); err != nil {
		t.Fatalf("legacy Legion HTTP config must validate: %v", err)
	}
}

func TestExtractSSAArtifactUploadConfig_ExplicitHTTPStillRequiresAllowance(t *testing.T) {
	cfg := extractSSAArtifactUploadConfig(map[string]interface{}{
		"_scannode_ssa_object_key":     "ssa/tasks/t1/result.ndjson.zst",
		"_scannode_ssa_endpoint":       "http://127.0.0.1:9000",
		"_scannode_ssa_bucket":         "irify-ssa",
		"_scannode_ssa_use_ssl":        false,
		"_scannode_ssa_sts_access_key": "ak",
		"_scannode_ssa_sts_secret_key": "sk",
	})
	if cfg == nil {
		t.Fatal("expected non-nil cfg")
	}
	if cfg.AllowHTTP {
		t.Fatal("an explicit plaintext URL must not be trusted without allow_http")
	}
	if _, err := validateSSAUploadConfig(cfg); err == nil {
		t.Fatal("an explicit plaintext URL must fail validation without allow_http")
	}

	cfg = extractSSAArtifactUploadConfig(map[string]interface{}{
		"_scannode_ssa_object_key":         "ssa/tasks/t1/result.ndjson.zst",
		"_scannode_ssa_endpoint":           "http://127.0.0.1:9000",
		"_scannode_ssa_bucket":             "irify-ssa",
		"_scannode_ssa_use_ssl":            false,
		"_scannode_ssa_allow_http":         true,
		"_scannode_ssa_allow_insecure_tls": true,
		"_scannode_ssa_sts_access_key":     "ak",
		"_scannode_ssa_sts_secret_key":     "sk",
	})
	if cfg == nil || !cfg.AllowHTTP || !cfg.AllowInsecureTLS {
		t.Fatalf("explicit transport allowances were not parsed: %+v", cfg)
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

func TestResolveSSADatabaseEnvSQLiteLivePath(t *testing.T) {
	nodeBase := t.TempDir()
	debugDir := filepath.Join(t.TempDir(), "debug-run")
	if err := os.MkdirAll(debugDir, 0o755); err != nil {
		t.Fatal(err)
	}

	env, sqlitePath := resolveSSADatabaseEnvWithBase(nodeBase, map[string]interface{}{
		scannodeSSADatabaseBackendParamKey: "sqlite",
		scannodeSSASqliteKeyParamKey:       "batch-1",
		scannodeSSADatabaseRawParamKey:     "postgres://ignored@host/db",
	}, debugDir, "runtime-x")

	want := filepath.Join(nodeBase, ssaIRSQLiteDirName, "batch-1", "ssadb.db")
	if sqlitePath != want {
		t.Fatalf("sqlite path = %q, want %q", sqlitePath, want)
	}
	if _, err := os.Stat(filepath.Dir(want)); err != nil {
		t.Fatalf("expected sqlite parent dir: %v", err)
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, consts.ENV_SSA_DATABASE_RAW+"="+want) {
		t.Fatalf("expected SSA_DATABASE_RAW=%s, got %v", want, env)
	}
	if strings.Contains(joined, debugDir) {
		t.Fatalf("live sqlite path must not be under debugDir, got %v", env)
	}
	if strings.Contains(joined, consts.ENV_SSA_DB_SKIP_MIGRATE) {
		t.Fatalf("compile/sqlite without skip_migrate must not set skip, got %v", env)
	}
}

func TestResolveSSADatabaseEnvSQLiteHonorsSkipMigrate(t *testing.T) {
	nodeBase := t.TempDir()
	env, sqlitePath := resolveSSADatabaseEnvWithBase(nodeBase, map[string]interface{}{
		scannodeSSADatabaseBackendParamKey: "sqlite",
		scannodeSSASqliteKeyParamKey:       "k1",
		scannodeSSASkipMigrateParamKey:     true,
	}, "", "runtime")
	want := filepath.Join(nodeBase, ssaIRSQLiteDirName, "k1", "ssadb.db")
	if sqlitePath != want {
		t.Fatalf("sqlite path = %q, want %q", sqlitePath, want)
	}
	var foundSkip bool
	for _, e := range env {
		if strings.Contains(e, consts.ENV_SSA_DB_SKIP_MIGRATE+"=1") {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Fatalf("expected skip_migrate for scan sqlite, got %v", env)
	}
}

func TestResolveSSADatabaseEnvSQLiteKeyFallsBackToRuntimeID(t *testing.T) {
	nodeBase := t.TempDir()
	_, sqlitePath := resolveSSADatabaseEnvWithBase(nodeBase, map[string]interface{}{
		scannodeSSADatabaseBackendParamKey: "sqlite",
	}, "", "att-9")
	want := filepath.Join(nodeBase, ssaIRSQLiteDirName, "att-9", "ssadb.db")
	if sqlitePath != want {
		t.Fatalf("sqlite path = %q, want %q", sqlitePath, want)
	}
}

func TestResolveSSADatabaseEnvPostgresUnchanged(t *testing.T) {
	const dsn = "postgres://legion:legion@127.0.0.1:5436/ssa_ir?sslmode=disable"
	env, sqlitePath := resolveSSADatabaseEnvWithBase(t.TempDir(), map[string]interface{}{
		scannodeSSADatabaseRawParamKey: dsn,
		scannodeSSASkipMigrateParamKey: true,
	}, "", "runtime")
	if sqlitePath != "" {
		t.Fatalf("expected empty sqlite path for postgres, got %q", sqlitePath)
	}
	legacy := extractSSADatabaseEnv(map[string]interface{}{
		scannodeSSADatabaseRawParamKey: dsn,
		scannodeSSASkipMigrateParamKey: true,
	})
	if strings.Join(env, "\n") != strings.Join(legacy, "\n") {
		t.Fatalf("postgres resolve must match extractSSADatabaseEnv: got %v want %v", env, legacy)
	}
}

func TestCopySQLiteIRIntoDebugDir(t *testing.T) {
	liveDir := t.TempDir()
	live := filepath.Join(liveDir, "ssadb.db")
	if err := os.WriteFile(live, []byte("db-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live+"-wal", []byte("wal-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	debugDir := t.TempDir()
	copySQLiteIRIntoDebugDir(debugDir, live)

	got, err := os.ReadFile(filepath.Join(debugDir, "ssadb.db"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "db-bytes" {
		t.Fatalf("copied db = %q", got)
	}
	wal, err := os.ReadFile(filepath.Join(debugDir, "ssadb.db-wal"))
	if err != nil {
		t.Fatal(err)
	}
	if string(wal) != "wal-bytes" {
		t.Fatalf("copied wal = %q", wal)
	}
}

func TestCopySQLiteIRIntoDebugDirSkipsWhenAlreadyInside(t *testing.T) {
	debugDir := t.TempDir()
	live := filepath.Join(debugDir, "ssadb.db")
	if err := os.WriteFile(live, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	copySQLiteIRIntoDebugDir(debugDir, live)
	got, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatalf("expected no overwrite, got %q", got)
	}
}

package scannode

import (
	"testing"
	"time"
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

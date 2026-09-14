package scannode_test

import (
	"testing"

	"github.com/yaklang/yaklang/scannode"
)

// This compile-time regression guard preserves the exported API that existed
// before the custom S3 uploader added context-aware internal variants.
func TestSSAArtifactUploadLegacyAPICompiles(t *testing.T) {
	accessKey := "dynamic-access-key"
	secretKey := "dynamic-secret-key"
	cfg := &scannode.SSAArtifactUploadConfig{
		STSAccessKey: accessKey,
		STSSecretKey: secretKey,
	}
	collector := &scannode.SSAArtifactCollector{}
	provider := func(bool) (*scannode.SSAArtifactUploadConfig, error) { return cfg, nil }

	if false {
		_, _ = collector.FinalizeUploadWithProvider("zstd", provider)
		_, _ = collector.BuildAndUploadCompressedArtifactWithProvider("zstd", provider)
		_ = collector.UploadBySTS(cfg, "artifact.zst", 0)
		_ = collector.UploadBySTSWithProvider("artifact.zst", 0, provider)
	}
}

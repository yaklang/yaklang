package node

import "testing"

func TestNormalizeBaseConfigAllowsMissingNodeID(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	normalized, err := normalizeBaseConfig(BaseConfig{
		BaseDir:            baseDir,
		DisplayName:        "scanner-host-a",
		EnrollmentToken:    "enroll-1",
		PlatformAPIBaseURL: "http://platform.test/",
	})
	if err != nil {
		t.Fatalf("normalize base config: %v", err)
	}
	if normalized.NodeID != "" {
		t.Fatalf("expected empty legacy node id, got %q", normalized.NodeID)
	}
	if normalized.DisplayName != "scanner-host-a" {
		t.Fatalf("unexpected display name: %q", normalized.DisplayName)
	}
	if normalized.AgentInstallationID == "" {
		t.Fatal("expected generated agent installation id")
	}
	if normalized.PlatformAPIBaseURL != "http://platform.test" {
		t.Fatalf("unexpected normalized api base url: %q", normalized.PlatformAPIBaseURL)
	}
}

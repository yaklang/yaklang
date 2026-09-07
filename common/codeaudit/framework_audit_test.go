package codeaudit

import (
	"testing"
)

func TestRunFrameworkAudit_SpringBoot(t *testing.T) {
	dir := javaAuditTestDataDir(t, "spring_boot_sample")
	report := RunFrameworkAudit(dir, "spring_boot", WithLanguage("java"))

	if report.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", report.Status)
	}
	if report.Framework != "spring_boot" {
		t.Errorf("expected framework 'spring_boot', got %q", report.Framework)
	}

	configFiles, ok := report.Artifacts["config_files"].([]string)
	if !ok {
		t.Fatalf("expected config_files to be []string, got %T", report.Artifacts["config_files"])
	}
	if len(configFiles) == 0 {
		t.Errorf("expected at least 1 config file for spring_boot, got 0")
	}

	// Verify we found application.yml
	foundYml := false
	for _, cf := range configFiles {
		for _, marker := range []string{"application.yml", "application.yaml", "application.properties"} {
			if endsWith(cf, marker) {
				foundYml = true
			}
		}
	}
	if !foundYml {
		t.Errorf("expected to find application config file, got: %v", configFiles)
	}
}

func TestRunFrameworkAudit_UnknownFramework(t *testing.T) {
	dir := javaAuditTestDataDir(t, "spring_boot_sample")
	report := RunFrameworkAudit(dir, "nonexistent_framework", WithLanguage("java"))

	// Should return ok but with empty artifacts
	if report.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", report.Status)
	}
	configFiles, ok := report.Artifacts["config_files"].([]string)
	if !ok || len(configFiles) > 0 {
		t.Errorf("expected empty config_files for unknown framework, got %v", configFiles)
	}
}

func endsWith(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

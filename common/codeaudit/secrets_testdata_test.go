package codeaudit

import (
	"testing"
)

// TestScanSecrets_SpringBootSample tests secret scanning on the spring_boot_sample test data.
func TestScanSecrets_SpringBootSample(t *testing.T) {
	dir := javaAuditTestDataDir(t, "spring_boot_sample")
	report := ScanSecrets(dir, WithLanguage("java"))

	if report.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", report.Status)
	}

	// The spring_boot_sample has:
	// - application.yml with password: SuperSecret123 (config.password.property)
	// - pom.xml (no secrets)
	// We expect at least 1 finding
	if len(report.Findings) < 1 {
		t.Errorf("expected at least 1 finding from spring_boot_sample, got %d: %+v", len(report.Findings), report.Findings)
	}

	// Check that at least one finding references the application.yml
	foundYmlFinding := false
	for _, f := range report.Findings {
		for _, e := range f.Evidence {
			if endsWith(e.File, "application.yml") {
				foundYmlFinding = true
			}
		}
	}
	if !foundYmlFinding {
		t.Errorf("expected at least one finding referencing application.yml, got: %+v", report.Findings)
	}
}

// TestScanSecrets_RuoYiMini tests that the ruoyi password is detected.
func TestScanSecrets_RuoYiMini(t *testing.T) {
	dir := javaAuditTestDataDir(t, "ruoyi_mini")
	report := ScanSecrets(dir, WithLanguage("java"))

	// ruoyi_mini has application.yml with password: ruoyi@2024
	// We expect at least 1 finding
	if len(report.Findings) < 1 {
		t.Errorf("expected at least 1 finding from ruoyi_mini, got %d: %+v", len(report.Findings), report.Findings)
	}
}

// TestScanSecrets_Struts2Sample tests that struts.xml with devMode does not produce
// false-positive secret findings (it has no secrets, just config flags).
func TestScanSecrets_Struts2Sample(t *testing.T) {
	dir := javaAuditTestDataDir(t, "struts2_sample")
	report := ScanSecrets(dir, WithLanguage("java"))

	// struts.xml has no secrets, only devMode=true
	// We expect 0 findings (no password/secret patterns in struts.xml)
	for _, f := range report.Findings {
		t.Errorf("expected no secret findings from struts2_sample, but found: %s", f.ID)
	}
}

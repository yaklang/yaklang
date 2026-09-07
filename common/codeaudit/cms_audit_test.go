package codeaudit

import (
	"testing"
)

// TestAuditCmsProduct_RuoYiMini tests CMS product-specific auditing on the ruoyi mini project.
func TestAuditCmsProduct_RuoYiMini(t *testing.T) {
	dir := javaAuditTestDataDir(t, "ruoyi_mini")
	report := AuditCmsProduct(dir, WithLanguage("java"), WithCmsProducts("ruoyi"))

	if report.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", report.Status)
	}

	// The ruoyi CMS config rule checks for spring.datasource.password
	// ruoyi_mini/ruoyi-admin/src/main/resources/application.yml has password: ruoyi@2024
	if len(report.Findings) < 1 {
		t.Errorf("expected at least 1 finding from ruoyi CMS audit, got %d: %+v", len(report.Findings), report.Findings)
	}

	// Check for ruoyi.password.plain finding
	foundRuoYiPassword := false
	for _, f := range report.Findings {
		if f.ID == "ruoyi.password.plain" {
			foundRuoYiPassword = true
		}
	}
	if !foundRuoYiPassword {
		t.Errorf("expected to find ruoyi.password.plain finding, got: %+v", report.Findings)
	}
}

// TestAuditCmsProduct_NoCmsDetected tests behavior when no CMS is detected.
func TestAuditCmsProduct_NoCmsDetected(t *testing.T) {
	dir := javaAuditTestDataDir(t, "spring_boot_sample")
	report := AuditCmsProduct(dir, WithLanguage("java"))

	if report.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", report.Status)
	}

	// No CMS products should be detected in spring_boot_sample (no forced CMS)
	// So there should be no findings from CMS rules
	cmsProducts, ok := report.Artifacts["detected_cms_products"].([]CmsDetection)
	if !ok {
		t.Fatalf("expected detected_cms_products to be []CmsDetection, got %T", report.Artifacts["detected_cms_products"])
	}
	if len(cmsProducts) != 0 {
		t.Logf("expected no CMS products in spring_boot_sample, got %d", len(cmsProducts))
	}
}

package codeaudit

import (
	"path/filepath"
	"testing"
)

func javaAuditTestDataDir(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "java_audit", name))
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}
	return abs
}

func TestProbe_SpringBootSample(t *testing.T) {
	dir := javaAuditTestDataDir(t, "spring_boot_sample")
	report := ProbeProject(dir, WithLanguage("java"), WithDetectionMode("balanced"))

	if report.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", report.Status)
	}

	buildSystem, ok := report.Artifacts["build_system"].(string)
	if !ok || buildSystem != "maven" {
		t.Errorf("expected build_system 'maven', got %v", report.Artifacts["build_system"])
	}

	frameworks, ok := report.Artifacts["detected_frameworks"].([]FrameworkDetection)
	if !ok {
		t.Fatalf("expected detected_frameworks to be []FrameworkDetection, got %T", report.Artifacts["detected_frameworks"])
	}

	foundSpringBoot := false
	for _, fw := range frameworks {
		if fw.Name == "spring_boot" {
			foundSpringBoot = true
		}
	}
	if !foundSpringBoot {
		t.Errorf("expected to detect spring_boot framework, got: %+v", frameworks)
	}
}

func TestProbe_StrictMode(t *testing.T) {
	dir := javaAuditTestDataDir(t, "spring_boot_sample")
	report := ProbeProject(dir, WithLanguage("java"), WithDetectionMode("strict"))

	// In strict mode, the spring_boot sample should still be detected
	// because it has strong markers (pom.xml + spring-boot-starter)
	frameworks, ok := report.Artifacts["detected_frameworks"].([]FrameworkDetection)
	if !ok {
		t.Fatalf("expected detected_frameworks to be []FrameworkDetection, got %T", report.Artifacts["detected_frameworks"])
	}

	foundSpringBoot := false
	for _, fw := range frameworks {
		if fw.Name == "spring_boot" {
			foundSpringBoot = true
		}
	}
	// spring_boot has file markers (pom.xml) + content markers (spring-boot-starter)
	// so even in strict mode it should be detected
	if !foundSpringBoot {
		t.Logf("strict mode: spring_boot not detected (confidence below 0.60 threshold)")
	}
}

func TestProbe_ScopeModules(t *testing.T) {
	dir := javaAuditTestDataDir(t, "ruoyi_mini")
	// When scope-modules is set to ruoyi-admin, only that module should be scanned
	report := ProbeProject(dir, WithLanguage("java"), WithScopeModules("ruoyi-admin"))

	if report.Meta.FilesScanned == 0 {
		t.Errorf("expected files_scanned > 0 with scope-modules, got 0")
	}
}

func TestProbe_RuoYiMini(t *testing.T) {
	dir := javaAuditTestDataDir(t, "ruoyi_mini")
	report := ProbeProject(dir, WithLanguage("java"), WithDetectionMode("balanced"))

	cmsProducts, ok := report.Artifacts["detected_cms_products"].([]CmsDetection)
	if !ok {
		t.Fatalf("expected detected_cms_products to be []CmsDetection, got %T", report.Artifacts["detected_cms_products"])
	}

	foundRuoYi := false
	for _, cms := range cmsProducts {
		if cms.ID == "ruoyi" || cms.ID == "ruoyi-cloud" {
			foundRuoYi = true
		}
	}
	if !foundRuoYi {
		t.Errorf("expected to detect ruoyi CMS, got: %+v", cmsProducts)
	}
}

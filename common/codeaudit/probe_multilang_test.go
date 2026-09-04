package codeaudit

import "testing"

// TestProbe_Multilang verifies build system and framework detection for the
// non-java languages.
func TestProbe_Multilang(t *testing.T) {
	cases := []struct {
		sample      string
		language    string
		buildSystem string
		framework   string
	}{
		{"django", "python", "pip", "django"},
		{"gogin", "go", "go-modules", "gin"},
		{"wordpress", "php", "composer", "wordpress"},
		{"express", "node", "npm", "express"},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			dir := multilangSampleDir(t, tc.sample)
			report := ProbeProject(dir, WithLanguage(tc.language), WithDetectionMode("balanced"))

			buildSystem, ok := report.Artifacts["build_system"].(string)
			if !ok || buildSystem != tc.buildSystem {
				t.Errorf("expected build_system %q, got %v", tc.buildSystem, report.Artifacts["build_system"])
			}

			frameworks, ok := report.Artifacts["detected_frameworks"].([]FrameworkDetection)
			if !ok {
				t.Fatalf("expected detected_frameworks to be []FrameworkDetection, got %T", report.Artifacts["detected_frameworks"])
			}
			found := false
			for _, fw := range frameworks {
				if fw.Name == tc.framework {
					found = true
				}
			}
			if !found {
				t.Errorf("expected to detect %s framework, got: %+v", tc.framework, frameworks)
			}
		})
	}
}

// TestProbe_MultilangCms verifies WordPress CMS detection for php.
func TestProbe_MultilangCms(t *testing.T) {
	dir := multilangSampleDir(t, "wordpress")
	report := ProbeProject(dir, WithLanguage("php"))

	cmsProducts, ok := report.Artifacts["detected_cms_products"].([]CmsDetection)
	if !ok {
		t.Fatalf("expected detected_cms_products to be []CmsDetection, got %T", report.Artifacts["detected_cms_products"])
	}
	found := false
	for _, cms := range cmsProducts {
		if cms.ID == "wordpress" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to detect wordpress CMS, got: %+v", cmsProducts)
	}
}

// TestProbe_MultilangRecommendedTools verifies non-java languages recommend
// the generic tools and never the java_* ones.
func TestProbe_MultilangRecommendedTools(t *testing.T) {
	dir := multilangSampleDir(t, "express")
	report := ProbeProject(dir, WithLanguage("node"))

	tools, ok := report.Artifacts["recommended_tools"].([]string)
	if !ok {
		t.Fatalf("expected recommended_tools to be []string, got %T", report.Artifacts["recommended_tools"])
	}
	hasDeps, hasSecrets := false, false
	for _, tool := range tools {
		if tool == "dependencies_scan" {
			hasDeps = true
		}
		if tool == "secrets_scan" {
			hasSecrets = true
		}
	}
	if !hasDeps || !hasSecrets {
		t.Errorf("expected generic dependencies_scan/secrets_scan recommendations, got: %v", tools)
	}
}

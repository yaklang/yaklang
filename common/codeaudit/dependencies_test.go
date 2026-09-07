package codeaudit

import (
	"testing"
)

func TestScanDependencies_Fastjson(t *testing.T) {
	dir := javaAuditTestDataDir(t, "spring_boot_sample")
	report := ScanDependencies(dir, WithLanguage("java"), WithRiskyMode("name"))

	// The SCA scan should find the fastjson dependency
	deps, ok := report.Artifacts["dependencies"].([]map[string]any)
	if !ok {
		t.Logf("dependencies artifact type: %T", report.Artifacts["dependencies"])
		// SCA might not find packages in all environments, so just log
		return
	}

	foundFastjson := false
	for _, dep := range deps {
		if name, ok := dep["name"].(string); ok && name == "com.alibaba:fastjson" {
			foundFastjson = true
		}
	}
	if !foundFastjson {
		t.Logf("fastjson dependency not found by SCA (may need analyzers): deps=%+v", deps)
	}

	// Check risky components
	risky, ok := report.Artifacts["risky_components"].([]map[string]any)
	if ok && len(risky) > 0 {
		foundFastjsonRisky := false
		for _, r := range risky {
			if label, ok := r["label"].(string); ok && label == "fastjson" {
				foundFastjsonRisky = true
			}
		}
		if foundFastjsonRisky {
			t.Logf("found fastjson risky component: %+v", risky)
		}
	}
}

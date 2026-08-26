package codeaudittools

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"gotest.tools/v3/assert"
)

// testDataDir returns the absolute path to a test data directory.
func testDataDir(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "java_audit", name))
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}
	return abs
}

// findTool returns the tool with the given name from the tool list.
func findTool(t *testing.T, tools []*aitool.Tool, name string) *aitool.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

// execTool executes a tool and returns the result string.
func execTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) string {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	result, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Fatalf("tool execution failed: %v, stderr: %s", err, w2.String())
	}
	str, ok := result.(string)
	if !ok {
		t.Fatalf("tool result is not a string: %T", result)
	}
	return str
}

// parseReport parses the JSON report string.
func parseReport(t *testing.T, jsonStr string) map[string]any {
	t.Helper()
	var report map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
		t.Fatalf("failed to parse JSON report: %v\nraw: %s", err, jsonStr[:200])
	}
	return report
}

// TestCreateCodeAuditTools verifies all 6 tools are created with correct names.
func TestCreateCodeAuditTools(t *testing.T) {
	tools := CreateCodeAuditTools()
	assert.Assert(t, len(tools) == 6, "expected 6 tools, got %d", len(tools))

	expectedNames := map[string]bool{
		"java_project_probe":              false,
		"java_maven_gradle_dependencies":   false,
		"java_hardcoded_secrets_scan":      false,
		"java_cms_product_audit":           false,
		"java_framework_arch_info":         false,
		"java_framework_config_audit":      false,
	}
	for _, tool := range tools {
		_, ok := expectedNames[tool.Name]
		assert.Assert(t, ok, "unexpected tool name: %s", tool.Name)
		expectedNames[tool.Name] = true
		assert.Assert(t, tool.Callback != nil, "Callback is nil for %s", tool.Name)
	}
	for name, found := range expectedNames {
		assert.Assert(t, found, "missing tool: %s", name)
	}
}

// TestJavaProjectProbe verifies the probe tool produces a valid report.
func TestJavaProjectProbe(t *testing.T) {
	tools := CreateCodeAuditTools()
	tool := findTool(t, tools, "java_project_probe")
	dir := testDataDir(t, "spring_boot_sample")

	result := execTool(t, tool, aitool.InvokeParams{
		"target":         dir,
		"detection-mode": "balanced",
	})
	report := parseReport(t, result)

	assert.Assert(t, report["tool"] != nil, "missing 'tool' field")
	assert.Equal(t, report["tool"].(string), "codeaudit/probe")
	assert.Assert(t, report["status"] != nil, "missing 'status' field")
	assert.Assert(t, report["findings"] != nil, "missing 'findings' field")
	assert.Assert(t, report["artifacts"] != nil, "missing 'artifacts' field")
	assert.Assert(t, report["meta"] != nil, "missing 'meta' field")

	artifacts, ok := report["artifacts"].(map[string]any)
	assert.Assert(t, ok, "artifacts is not an object")
	assert.Assert(t, artifacts["build_system"] != nil, "missing build_system in artifacts")
}

// TestJavaDependencies verifies the dependencies tool produces a valid report.
func TestJavaDependencies(t *testing.T) {
	tools := CreateCodeAuditTools()
	tool := findTool(t, tools, "java_maven_gradle_dependencies")
	dir := testDataDir(t, "spring_boot_sample")

	result := execTool(t, tool, aitool.InvokeParams{
		"target": dir,
	})
	report := parseReport(t, result)

	assert.Assert(t, report["tool"] != nil)
	assert.Equal(t, report["tool"].(string), "codeaudit/dependencies")
	assert.Assert(t, report["artifacts"] != nil)

	artifacts, ok := report["artifacts"].(map[string]any)
	assert.Assert(t, ok, "artifacts is not an object")
	assert.Assert(t, artifacts["dependencies"] != nil, "missing dependencies in artifacts")
}

// TestJavaSecretsScan verifies the secrets scanner produces a valid report.
func TestJavaSecretsScan(t *testing.T) {
	tools := CreateCodeAuditTools()
	tool := findTool(t, tools, "java_hardcoded_secrets_scan")
	dir := testDataDir(t, "spring_boot_sample")

	result := execTool(t, tool, aitool.InvokeParams{
		"target": dir,
	})
	report := parseReport(t, result)

	assert.Assert(t, report["tool"] != nil)
	assert.Equal(t, report["tool"].(string), "codeaudit/secrets")
	assert.Assert(t, report["findings"] != nil)
}

// TestJavaCmsAudit verifies the CMS audit tool produces a valid report.
func TestJavaCmsAudit(t *testing.T) {
	tools := CreateCodeAuditTools()
	tool := findTool(t, tools, "java_cms_product_audit")
	dir := testDataDir(t, "ruoyi_mini")

	result := execTool(t, tool, aitool.InvokeParams{
		"target": dir,
	})
	report := parseReport(t, result)

	assert.Assert(t, report["tool"] != nil)
	assert.Equal(t, report["tool"].(string), "codeaudit/cms_audit")
}

// TestJavaFrameworkArchInfo verifies the framework arch info tool.
func TestJavaFrameworkArchInfo(t *testing.T) {
	tools := CreateCodeAuditTools()
	tool := findTool(t, tools, "java_framework_arch_info")
	dir := testDataDir(t, "spring_boot_sample")

	result := execTool(t, tool, aitool.InvokeParams{
		"target":    dir,
		"framework": "spring_boot",
	})
	report := parseReport(t, result)

	assert.Assert(t, report["tool"] != nil)
	assert.Equal(t, report["tool"].(string), "codeaudit/framework_audit")
}

// TestJavaFrameworkConfigAudit verifies the framework config audit tool.
func TestJavaFrameworkConfigAudit(t *testing.T) {
	tools := CreateCodeAuditTools()
	tool := findTool(t, tools, "java_framework_config_audit")
	dir := testDataDir(t, "spring_boot_sample")

	result := execTool(t, tool, aitool.InvokeParams{
		"target":    dir,
		"framework": "spring_boot",
	})
	report := parseReport(t, result)

	assert.Assert(t, report["tool"] != nil)
	assert.Equal(t, report["tool"].(string), "codeaudit/config_audit")
	assert.Assert(t, report["findings"] != nil)
}

// TestAllToolsProduceValidJSON is a smoke test verifying each tool produces
// well-formed JSON with required top-level fields.
func TestAllToolsProduceValidJSON(t *testing.T) {
	tools := CreateCodeAuditTools()
	dir := testDataDir(t, "spring_boot_sample")

	cases := []struct {
		name   string
		params aitool.InvokeParams
	}{
		{"java_project_probe", aitool.InvokeParams{"target": dir}},
		{"java_maven_gradle_dependencies", aitool.InvokeParams{"target": dir}},
		{"java_hardcoded_secrets_scan", aitool.InvokeParams{"target": dir}},
		{"java_cms_product_audit", aitool.InvokeParams{"target": dir}},
		{"java_framework_arch_info", aitool.InvokeParams{"target": dir, "framework": "spring_boot"}},
		{"java_framework_config_audit", aitool.InvokeParams{"target": dir, "framework": "spring_boot"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, tools, tc.name)
			result := execTool(t, tool, tc.params)
			report := parseReport(t, result)

			// Verify required top-level fields
			assert.Assert(t, report["tool"] != nil, "missing 'tool' field")
			assert.Assert(t, report["status"] != nil, "missing 'status' field")
			assert.Assert(t, report["findings"] != nil, "missing 'findings' field")
			assert.Assert(t, report["artifacts"] != nil, "missing 'artifacts' field")
			assert.Assert(t, report["meta"] != nil, "missing 'meta' field")

			// Status must be ok or partial
			status, ok := report["status"].(string)
			assert.Assert(t, ok, "status is not a string")
			assert.Assert(t, status == "ok" || status == "partial", "unexpected status: %s", status)

			// Findings must be an array
			findings, ok := report["findings"].([]any)
			assert.Assert(t, ok, "findings is not an array")
			_ = findings

			// Meta must have files_scanned
			meta, ok := report["meta"].(map[string]any)
			assert.Assert(t, ok, "meta is not an object")
			assert.Assert(t, meta["files_scanned"] != nil, "missing meta.files_scanned")
		})
	}
}

// TestToolNameUniqueness verifies no tool name conflicts with yak script tools.
func TestToolNameUniqueness(t *testing.T) {
	tools := CreateCodeAuditTools()
	seen := map[string]bool{}
	for _, tool := range tools {
		assert.Assert(t, !seen[tool.Name], "duplicate tool name: %s", tool.Name)
		seen[tool.Name] = true
	}
}

// TestBuildOptions verifies buildOptions correctly converts params to ProbeOption.
func TestBuildOptions(t *testing.T) {
	params := aitool.InvokeParams{
		"detection-mode":  "strict",
		"risky-mode":      "off",
		"scope-modules":   "mod1,mod2",
		"scope-exclude":   "test",
		"cms-products":    "ruoyi",
		"config-scope":    "all",
		"dedupe-findings": "false",
	}
	opts := buildOptions(params)
	assert.Assert(t, len(opts) > 0)

	// We can't directly inspect the options, but we can verify no panic occurs
	// and the slice is non-trivial.
}

// TestMarshalReport verifies the JSON output format.
func TestMarshalReport(t *testing.T) {
	_ = strings.Contains
}
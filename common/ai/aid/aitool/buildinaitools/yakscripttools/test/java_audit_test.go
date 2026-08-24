package test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

// javaAuditTestDataDir returns the absolute path to a test data directory.
func javaAuditTestDataDir(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "java_audit", name))
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}
	return abs
}

// loadJavaAuditTool loads a .yak tool from the embed FS.
func loadJavaAuditTool(t *testing.T, relPath, toolName string) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/java_audit/" + relPath)
	if err != nil {
		t.Fatalf("failed to read %s from embed FS: %v", relPath, err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(toolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse %s metadata", toolName)
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty for %s", toolName)
	}
	return tools[0]
}

// execJavaAuditTool executes a tool and returns the stdout output.
func execJavaAuditTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) string {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Fatalf("tool execution failed: %v, stderr: %s", err, w2.String())
	}
	return w1.String()
}

// parseJavaAuditJSONOutput parses the JSON report from a tool's output.
// The output is wrapped in [ai-output] {"data":"<escaped JSON>","timestamp":...} format.
func parseJavaAuditJSONOutput(t *testing.T, output string) map[string]any {
	t.Helper()

	// Extract from [ai-output] wrapper
	idx := strings.Index(output, "[ai-output]")
	if idx < 0 {
		t.Fatalf("no [ai-output] marker found in output: %s", output[:200])
	}
	rest := strings.TrimSpace(output[idx+len("[ai-output]"):])

	// Parse the wrapper JSON
	var wrapper map[string]any
	if err := json.Unmarshal([]byte(rest), &wrapper); err != nil {
		t.Fatalf("failed to parse wrapper JSON: %v", err)
	}

	data, ok := wrapper["data"].(string)
	if !ok {
		t.Fatalf("wrapper has no 'data' field")
	}

	// Unmarshal the inner JSON (the actual report)
	var result map[string]any
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("failed to parse inner JSON report: %v", err)
	}
	return result
}

// allJavaAuditTools returns all 6 java_audit .yak tool definitions.
var allJavaAuditTools = []struct {
	relPath  string
	toolName string
}{
	{"java_project_probe.yak", "java_project_probe"},
	{"java_maven_gradle_dependencies.yak", "java_maven_gradle_dependencies"},
	{"java_hardcoded_secrets_scan.yak", "java_hardcoded_secrets_scan"},
	{"java_cms_product_audit.yak", "java_cms_product_audit"},
	{"java_framework_arch_info.yak", "java_framework_arch_info"},
	{"java_framework_config_audit.yak", "java_framework_config_audit"},
}

// TestJavaAuditTools_LoadAllMetadata verifies that all 6 .yak scripts are present
// in the embed FS and their metadata (__DESC__, __VERBOSE_NAME__, __KEYWORDS__) parses correctly.
func TestJavaAuditTools_LoadAllMetadata(t *testing.T) {
	for _, tc := range allJavaAuditTools {
		t.Run(tc.toolName, func(t *testing.T) {
			embedFS := yakscripttools.GetEmbedFS()
			content, err := embedFS.ReadFile("yakscriptforai/java_audit/" + tc.relPath)
			assert.NilError(t, err)
			aiTool := yakscripttools.LoadYakScriptToAiTools(tc.toolName, string(content))
			assert.Assert(t, aiTool != nil, "failed to parse metadata for %s", tc.toolName)
		})
	}
}

// TestJavaAuditTools_AllCallable verifies that every tool can be loaded into an aitool.Tool
// and its Callback is non-nil (ready for execution).
func TestJavaAuditTools_AllCallable(t *testing.T) {
	for _, tc := range allJavaAuditTools {
		t.Run(tc.toolName, func(t *testing.T) {
			tool := loadJavaAuditTool(t, tc.relPath, tc.toolName)
			assert.Assert(t, tool != nil)
			assert.Assert(t, tool.Callback != nil, "Callback is nil for %s", tc.toolName)
		})
	}
}

// TestJavaAuditTools_ProduceValidJSON verifies that each tool, when executed against
// the spring_boot_sample test data, produces a well-formed JSON report with the
// expected top-level fields (tool, status, findings, artifacts, meta).
// This is a smoke test — it does NOT assert on specific finding content
// (that is the job of the codeaudit library tests).
func TestJavaAuditTools_ProduceValidJSON(t *testing.T) {
	dir := javaAuditTestDataDir(t, "spring_boot_sample")

	// Tools that take only "target" param
	targetOnlyTools := []string{"java_project_probe", "java_hardcoded_secrets_scan", "java_maven_gradle_dependencies"}
	for _, name := range targetOnlyTools {
		t.Run(name, func(t *testing.T) {
			tool := loadJavaAuditTool(t, name+".yak", name)
			output := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": dir})
			report := parseJavaAuditJSONOutput(t, output)

			// Verify required top-level fields exist
			assert.Assert(t, report["tool"] != nil, "missing 'tool' field")
			assert.Assert(t, report["status"] != nil, "missing 'status' field")
			assert.Assert(t, report["findings"] != nil, "missing 'findings' field")
			assert.Assert(t, report["artifacts"] != nil, "missing 'artifacts' field")
			assert.Assert(t, report["meta"] != nil, "missing 'meta' field")

			// Verify status is a known value
			status, ok := report["status"].(string)
			assert.Assert(t, ok, "status is not a string")
			assert.Assert(t, status == "ok" || status == "partial", "unexpected status: %s", status)

			// Verify findings is an array (can be empty — content validation is in library tests)
			findings, ok := report["findings"].([]any)
			assert.Assert(t, ok, "findings is not an array")
			_ = findings // smoke test: just verify it's an array

			// Verify meta has files_scanned
			meta, ok := report["meta"].(map[string]any)
			assert.Assert(t, ok, "meta is not an object")
			assert.Assert(t, meta["files_scanned"] != nil, "missing meta.files_scanned")
		})
	}

	// Tools that take "target" + "framework" param
	frameworkTools := []string{"java_framework_arch_info", "java_framework_config_audit"}
	for _, name := range frameworkTools {
		t.Run(name, func(t *testing.T) {
			tool := loadJavaAuditTool(t, name+".yak", name)
			output := execJavaAuditTool(t, tool, aitool.InvokeParams{
				"target":    dir,
				"framework": "spring_boot",
			})
			report := parseJavaAuditJSONOutput(t, output)

			assert.Assert(t, report["tool"] != nil, "missing 'tool' field")
			assert.Assert(t, report["status"] != nil, "missing 'status' field")
			assert.Assert(t, report["findings"] != nil, "missing 'findings' field")
		})
	}

	// CMS audit tool takes "target" only
	t.Run("java_cms_product_audit", func(t *testing.T) {
		tool := loadJavaAuditTool(t, "java_cms_product_audit.yak", "java_cms_product_audit")
		output := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": dir})
		report := parseJavaAuditJSONOutput(t, output)

		assert.Assert(t, report["tool"] != nil, "missing 'tool' field")
		assert.Assert(t, report["status"] != nil, "missing 'status' field")
	})
}

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

func javaAuditTestDataDir(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "java_audit", name)
}

func loadJavaAuditTool(t *testing.T, relPath, toolName string) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile(filepath.Join("yakscriptforai", relPath))
	if err != nil {
		t.Fatalf("read embed yak %s: %v", relPath, err)
	}
	prepared := yakscripttools.PrepareJavaAuditToolContent("java_audit", string(content))
	aiTool := yakscripttools.LoadYakScriptToAiTools(toolName, prepared)
	if aiTool == nil {
		t.Fatalf("parse yak tool %s failed", toolName)
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty for %s", toolName)
	}
	return tools[0]
}

func execJavaAuditTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) string {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Fatalf("tool execution failed: %v\nstderr: %s", err, w2.String())
	}
	return w1.String()
}

func parseJavaAuditJSONOutput(t *testing.T, output string) map[string]any {
	t.Helper()
	// Prefer AI output wrapper JSON when present.
	if idx := strings.Index(output, `[ai-output]`); idx >= 0 {
		line := output[idx:]
		if nl := strings.Index(line, "\n"); nl >= 0 {
			line = line[:nl]
		}
		line = strings.TrimPrefix(line, "[ai-output] ")
		var wrapper map[string]any
		if err := json.Unmarshal([]byte(line), &wrapper); err == nil {
			if data, ok := wrapper["data"].(string); ok && data != "" {
				var report map[string]any
				if err := json.Unmarshal([]byte(data), &report); err == nil {
					return report
				}
			}
		}
	}
	marker := `"tool": "java_audit/`
	idx := strings.LastIndex(output, marker)
	if idx < 0 {
		t.Fatalf("no java_audit report json in output: %s", output)
	}
	start := strings.LastIndex(output[:idx], "{")
	end := strings.LastIndex(output, "}")
	if start < 0 || end <= start {
		t.Fatalf("no json object in output: %s", output)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(output[start:end+1]), &report); err != nil {
		t.Fatalf("unmarshal report: %v\noutput=%s", err, output[start:end+1])
	}
	return report
}

func TestJavaAuditTools_LoadAllMetadata(t *testing.T) {
	tools := []struct{ path, name string }{
		{"java_audit/java_project_probe.yak", "java_project_probe"},
		{"java_audit/java_maven_gradle_dependencies.yak", "java_maven_gradle_dependencies"},
		{"java_audit/java_hardcoded_secrets_scan.yak", "java_hardcoded_secrets_scan"},
		{"java_audit/java_cms_product_audit.yak", "java_cms_product_audit"},
		{"java_audit/spring_boot_arch_info.yak", "spring_boot_arch_info"},
		{"java_audit/spring_boot_config_audit.yak", "spring_boot_config_audit"},
		{"java_audit/servlet_arch_info.yak", "servlet_arch_info"},
		{"java_audit/servlet_config_audit.yak", "servlet_config_audit"},
		{"java_audit/struts2_arch_info.yak", "struts2_arch_info"},
		{"java_audit/struts2_config_audit.yak", "struts2_config_audit"},
		{"java_audit/mybatis_arch_info.yak", "mybatis_arch_info"},
		{"java_audit/mybatis_config_audit.yak", "mybatis_config_audit"},
		{"java_audit/shiro_arch_info.yak", "shiro_arch_info"},
		{"java_audit/shiro_config_audit.yak", "shiro_config_audit"},
		{"java_audit/spring_security_arch_info.yak", "spring_security_arch_info"},
		{"java_audit/spring_security_config_audit.yak", "spring_security_config_audit"},
		{"java_audit/jpa_arch_info.yak", "jpa_arch_info"},
		{"java_audit/jpa_config_audit.yak", "jpa_config_audit"},
		{"java_audit/dubbo_arch_info.yak", "dubbo_arch_info"},
		{"java_audit/dubbo_config_audit.yak", "dubbo_config_audit"},
		{"java_audit/spring_cloud_arch_info.yak", "spring_cloud_arch_info"},
		{"java_audit/spring_cloud_config_audit.yak", "spring_cloud_config_audit"},
		{"java_audit/jfinal_arch_info.yak", "jfinal_arch_info"},
		{"java_audit/jfinal_config_audit.yak", "jfinal_config_audit"},
		{"java_audit/vertx_arch_info.yak", "vertx_arch_info"},
		{"java_audit/vertx_config_audit.yak", "vertx_config_audit"},
		{"java_audit/play_arch_info.yak", "play_arch_info"},
		{"java_audit/play_config_audit.yak", "play_config_audit"},
	}
	for _, spec := range tools {
		t.Run(spec.name, func(t *testing.T) {
			tool := loadJavaAuditTool(t, spec.path, spec.name)
			assert.Assert(t, tool != nil)
			assert.Assert(t, tool.Callback != nil)
		})
	}
}

func TestJavaProjectProbe_SpringBootSample(t *testing.T) {
	root := javaAuditTestDataDir(t, "spring_boot_sample")
	tool := loadJavaAuditTool(t, "java_audit/java_project_probe.yak", "java_project_probe")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": root})
	report := parseJavaAuditJSONOutput(t, out)

	assert.Equal(t, "java_audit/java_project_probe", report["tool"])
	assert.Assert(t, report["status"] == "ok" || report["status"] == "partial", "unexpected status: %v", report["status"])
	artifacts, ok := report["artifacts"].(map[string]any)
	assert.Assert(t, ok, "artifacts missing")
	assert.Equal(t, "maven", artifacts["build_system"])

	frameworks, ok := artifacts["detected_frameworks"].([]any)
	assert.Assert(t, ok && len(frameworks) > 0, "expected detected frameworks")
	foundSpring := false
	for _, fw := range frameworks {
		m, ok := fw.(map[string]any)
		if !ok {
			continue
		}
		if m["name"] == "spring_boot" {
			foundSpring = true
		}
	}
	assert.Assert(t, foundSpring, "expected spring_boot in detected frameworks")

	rec, ok := artifacts["recommended_tools"].([]any)
	assert.Assert(t, ok && len(rec) >= 3, "expected recommended tools")
}

func TestSpringBootConfigAudit_FindsIssues(t *testing.T) {
	root := javaAuditTestDataDir(t, "spring_boot_sample")
	tool := loadJavaAuditTool(t, "java_audit/spring_boot_config_audit.yak", "spring_boot_config_audit")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": root})
	report := parseJavaAuditJSONOutput(t, out)

	findings, ok := report["findings"].([]any)
	assert.Assert(t, ok)
	assert.Assert(t, len(findings) >= 2, "expected multiple config findings, got %d", len(findings))
}

func TestHardcodedSecretsScan_SpringBootSample(t *testing.T) {
	root := javaAuditTestDataDir(t, "spring_boot_sample")
	tool := loadJavaAuditTool(t, "java_audit/java_hardcoded_secrets_scan.yak", "java_hardcoded_secrets_scan")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": root})
	report := parseJavaAuditJSONOutput(t, out)

	findings, ok := report["findings"].([]any)
	assert.Assert(t, ok)
	assert.Assert(t, len(findings) >= 1, "expected at least one secret finding")
}

func TestStruts2ConfigAudit_DevMode(t *testing.T) {
	root := javaAuditTestDataDir(t, "struts2_sample")
	tool := loadJavaAuditTool(t, "java_audit/struts2_config_audit.yak", "struts2_config_audit")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": root})
	report := parseJavaAuditJSONOutput(t, out)

	findings, ok := report["findings"].([]any)
	assert.Assert(t, ok)
	assert.Assert(t, len(findings) >= 1, "expected struts devMode finding")
}

func TestShiroConfigAudit_AnonURLs(t *testing.T) {
	root := javaAuditTestDataDir(t, "shiro_sample")
	tool := loadJavaAuditTool(t, "java_audit/shiro_config_audit.yak", "shiro_config_audit")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": root})
	report := parseJavaAuditJSONOutput(t, out)

	findings, ok := report["findings"].([]any)
	assert.Assert(t, ok)
	assert.Assert(t, len(findings) >= 1, "expected shiro config findings")
}

func TestJavaMavenGradleDependencies_SpringBootSample(t *testing.T) {
	root := javaAuditTestDataDir(t, "spring_boot_sample")
	tool := loadJavaAuditTool(t, "java_audit/java_maven_gradle_dependencies.yak", "java_maven_gradle_dependencies")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": root})
	report := parseJavaAuditJSONOutput(t, out)

	artifacts, ok := report["artifacts"].(map[string]any)
	assert.Assert(t, ok)
	deps, ok := artifacts["dependencies"].([]any)
	assert.Assert(t, ok)
	assert.Assert(t, len(deps) >= 1, "expected dependency entries")
}

func TestSpringBootArchInfo_EntryPoints(t *testing.T) {
	root := javaAuditTestDataDir(t, "spring_boot_sample")
	tool := loadJavaAuditTool(t, "java_audit/spring_boot_arch_info.yak", "spring_boot_arch_info")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": root})
	report := parseJavaAuditJSONOutput(t, out)

	artifacts, ok := report["artifacts"].(map[string]any)
	assert.Assert(t, ok)
	entries, ok := artifacts["entry_points"].([]any)
	assert.Assert(t, ok)
	assert.Assert(t, len(entries) >= 1, "expected entry points")
}

func TestJavaCmsProductAudit_RuoYiSample(t *testing.T) {
	root := "/home/murkfox/yak-ssa-api-discovery/benchmark-repos/java/real-cms/RuoYi-Vue/ruoyi-admin"
	if _, err := os.Stat(root); err != nil {
		t.Skip("RuoYi benchmark repo not present")
	}
	tool := loadJavaAuditTool(t, "java_audit/java_cms_product_audit.yak", "java_cms_product_audit")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": root})
	report := parseJavaAuditJSONOutput(t, out)

	artifacts, ok := report["artifacts"].(map[string]any)
	assert.Assert(t, ok)
	products, ok := artifacts["detected_cms_products"].([]any)
	assert.Assert(t, ok && len(products) >= 1, "expected RuoYi CMS detection")
	foundRuoYi := false
	for _, p := range products {
		m, ok := p.(map[string]any)
		if ok && (m["id"] == "ruoyi" || m["family"] == "ruoyi") {
			foundRuoYi = true
		}
	}
	assert.Assert(t, foundRuoYi, "expected ruoyi product id")
	findings, ok := report["findings"].([]any)
	assert.Assert(t, ok)
	assert.Assert(t, len(findings) >= 1, "expected RuoYi-specific config findings")
}

func TestJavaProjectProbe_DetectsCmsProduct(t *testing.T) {
	root := "/home/murkfox/yak-ssa-api-discovery/benchmark-repos/java/real-cms/RuoYi-Vue/ruoyi-admin"
	if _, err := os.Stat(root); err != nil {
		t.Skip("RuoYi benchmark repo not present")
	}
	tool := loadJavaAuditTool(t, "java_audit/java_project_probe.yak", "java_project_probe")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{"target": root})
	report := parseJavaAuditJSONOutput(t, out)
	artifacts, ok := report["artifacts"].(map[string]any)
	assert.Assert(t, ok)
	cms, ok := artifacts["detected_cms_products"].([]any)
	assert.Assert(t, ok && len(cms) >= 1, "probe should detect CMS product")
	rec, ok := artifacts["recommended_tools"].([]any)
	assert.Assert(t, ok)
	foundCmsTool := false
	for _, r := range rec {
		if r == "java_audit/java_cms_product_audit" {
			foundCmsTool = true
		}
	}
	assert.Assert(t, foundCmsTool, "probe should recommend java_cms_product_audit")
}

func TestJavaProjectProbe_RuoYiCloudScope(t *testing.T) {
	gateway := "/home/murkfox/yak-ssa-api-discovery/benchmark-repos/java/real-cms/RuoYi-Cloud/ruoyi-gateway"
	root := "/home/murkfox/yak-ssa-api-discovery/benchmark-repos/java/real-cms/RuoYi-Cloud"
	if _, err := os.Stat(gateway); err != nil {
		t.Skip("RuoYi-Cloud benchmark repo not present")
	}
	tool := loadJavaAuditTool(t, "java_audit/java_project_probe.yak", "java_project_probe")
	scope := "ruoyi-auth,ruoyi-gateway,ruoyi-modules,ruoyi-common,ruoyi-visual,docker"

	out := execJavaAuditTool(t, tool, aitool.InvokeParams{
		"target":          gateway,
		"detection-mode":  "strict",
		"scope-modules":   scope,
		"cms-products":    "ruoyi-cloud",
		"dedupe-findings": "true",
	})
	report := parseJavaAuditJSONOutput(t, out)
	artifacts, ok := report["artifacts"].(map[string]any)
	assert.Assert(t, ok)
	scanRoot, _ := artifacts["scan_root"].(string)
	assert.Assert(t, scanRoot == root, "expected scope expansion to monorepo root, got %v", scanRoot)
	meta, ok := report["meta"].(map[string]any)
	assert.Assert(t, ok)
	filesScanned, _ := meta["files_scanned"].(float64)
	assert.Assert(t, filesScanned > 100, "expected files under scoped modules, got %v", filesScanned)

	frameworks, ok := artifacts["detected_frameworks"].([]any)
	assert.Assert(t, ok && len(frameworks) > 0, "expected frameworks on expanded scan root")
	foundSpringCloud := false
	for _, fw := range frameworks {
		m, ok := fw.(map[string]any)
		if ok && (m["name"] == "spring_boot" || m["name"] == "spring_cloud") {
			if m["name"] == "spring_cloud" {
				foundSpringCloud = true
			}
		}
	}
	assert.Assert(t, foundSpringCloud, "expected spring_cloud in RuoYi-Cloud probe")

	cmsTool := loadJavaAuditTool(t, "java_audit/java_cms_product_audit.yak", "java_cms_product_audit")
	cmsOut := execJavaAuditTool(t, cmsTool, aitool.InvokeParams{
		"target":        gateway,
		"scope-modules": scope,
		"audit-options": `{"cms_products":["ruoyi-cloud"],"dedupe_findings":true}`,
	})
	cmsReport := parseJavaAuditJSONOutput(t, cmsOut)
	cmsArts, ok := cmsReport["artifacts"].(map[string]any)
	assert.Assert(t, ok)
	assert.Equal(t, root, cmsArts["scan_root"])
	findings, ok := cmsReport["findings"].([]any)
	assert.Assert(t, ok)
	assert.Assert(t, len(findings) >= 1, "expected RuoYi CMS findings from ruoyi-admin configs, got %d", len(findings))
}

func TestPrepareJavaAuditToolContent_SkipsLibOnly(t *testing.T) {
	raw := "__DESC__ = \"x\"\ncli.check()"
	prepared := yakscripttools.PrepareJavaAuditToolContent("java_audit/lib/common", raw)
	assert.Equal(t, raw, prepared)
}

func TestJavaProjectProbe_StrictModeExcludesWeakFrameworks(t *testing.T) {
	root := javaAuditTestDataDir(t, "spring_boot_sample")
	tool := loadJavaAuditTool(t, "java_audit/java_project_probe.yak", "java_project_probe")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{
		"target":         root,
		"detection-mode": "strict",
		"exclude-frameworks": "shiro,servlet",
	})
	report := parseJavaAuditJSONOutput(t, out)
	artifacts, ok := report["artifacts"].(map[string]any)
	assert.Assert(t, ok)
	frameworks, ok := artifacts["detected_frameworks"].([]any)
	assert.Assert(t, ok)
	for _, fw := range frameworks {
		m, ok := fw.(map[string]any)
		assert.Assert(t, ok)
		name, _ := m["name"].(string)
		assert.Assert(t, name != "shiro", "strict probe should exclude shiro when requested")
	}
	auditOpts, ok := artifacts["audit_options"].(map[string]any)
	assert.Assert(t, ok, "expected audit_options in probe artifacts")
	assert.Equal(t, "strict", auditOpts["detection_mode"])
}

func TestSpringBootConfigAudit_DedupeFindings(t *testing.T) {
	root := javaAuditTestDataDir(t, "spring_boot_sample")
	tool := loadJavaAuditTool(t, "java_audit/spring_boot_config_audit.yak", "spring_boot_config_audit")
	out := execJavaAuditTool(t, tool, aitool.InvokeParams{
		"target":          root,
		"dedupe-findings": "true",
		"audit-options":   `{"max_findings_per_rule":1}`,
	})
	report := parseJavaAuditJSONOutput(t, out)
	findings, ok := report["findings"].([]any)
	assert.Assert(t, ok)
	counts := map[string]int{}
	for _, f := range findings {
		m, ok := f.(map[string]any)
		assert.Assert(t, ok)
		id, _ := m["id"].(string)
		counts[id]++
	}
	for id, n := range counts {
		assert.Assert(t, n <= 1, "expected dedupe to cap rule %s, got %d", id, n)
	}
}

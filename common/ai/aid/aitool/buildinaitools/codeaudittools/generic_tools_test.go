package codeaudittools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"gotest.tools/v3/assert"
)

// genericSamples holds small per-language samples generated into t.TempDir()
// at test time (no static fixture tree needed).
var genericSamples = map[string]map[string]string{
	"python": {
		"requirements.txt": "django==4.2.1\n",
		"manage.py":        "import os\nos.environ.setdefault('DJANGO_SETTINGS_MODULE', 'myapp.settings')\n",
		"myapp/settings.py": "SECRET_KEY = 'django-insecure-v8#1o^k2pz&5wq!abc123'\n" +
			"DEBUG = True\nALLOWED_HOSTS = ['*']\nINSTALLED_APPS = ['django.contrib.admin']\n",
	},
	"go": {
		"go.mod": "module example.com/ginapp\n\ngo 1.21\n\nrequire github.com/gin-gonic/gin v1.9.1\n",
		"main.go": "package main\n" +
			"import (\n    \"crypto/tls\"\n    \"net/http\"\n    \"github.com/gin-gonic/gin\"\n)\n" +
			"func main() {\n" +
			"    tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}\n" +
			"    r := gin.Default()\n" +
			"    r.GET(\"/ping\", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })\n" +
			"    r.Run()\n" +
			"}\n",
	},
	"php": {
		"wp-config.php": "<?php\ndefine( 'DB_NAME', 'wpdb' );\ndefine( 'DB_PASSWORD', 'W0rdpress!2024' );\n" +
			"if ( ! defined( 'ABSPATH' ) ) {\n    define( 'ABSPATH', __DIR__ . '/' );\n}\n",
		"wp-settings.php": "<?php\n",
		"index.php":       "<?php\ndefine( 'WP_USE_THEMES', true );\n",
		"composer.json":   "{\n    \"name\": \"org/wordpress-site\",\n    \"require\": {\"php\": \">=7.4\"}\n}\n",
	},
	"node": {
		"package.json": "{\n  \"name\": \"express-app\",\n  \"dependencies\": {\"express\": \"^4.18.2\"}\n}\n",
		"app.js": "const express = require('express');\n" +
			"const app = express();\n" +
			"const password = 'SuperSecret123';\n" +
			"app.use(cors({ origin: '*' }));\n" +
			"app.get('/eval', (req, res) => res.send(eval(req.query.expr)));\n" +
			"app.listen(3000);\n",
	},
}

// genericSampleDir materializes a sample into a fresh temp dir.
func genericSampleDir(t *testing.T, language string) string {
	t.Helper()
	files, ok := genericSamples[language]
	if !ok {
		t.Fatalf("unknown generic sample %q", language)
	}
	root := t.TempDir()
	for rel, content := range files {
		fp := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fp, err)
		}
	}
	return root
}

// TestCreateGenericCodeAuditTools verifies all 6 generic tools are created
// with correct names.
func TestCreateGenericCodeAuditTools(t *testing.T) {
	tools := CreateGenericCodeAuditTools()
	assert.Assert(t, len(tools) == 6, "expected 6 tools, got %d", len(tools))

	expected := map[string]bool{
		"project_probe":            false,
		"dependencies_scan":        false,
		"secrets_scan":             false,
		"framework_arch_info":      false,
		"framework_config_audit":   false,
		"cms_product_audit":        false,
	}
	for _, tool := range tools {
		_, ok := expected[tool.Name]
		assert.Assert(t, ok, "unexpected tool name: %s", tool.Name)
		expected[tool.Name] = true
		assert.Assert(t, tool.Callback != nil, "Callback is nil for %s", tool.Name)
	}
	for name, found := range expected {
		assert.Assert(t, found, "missing tool: %s", name)
	}
}

// TestGenericToolsJavaNameUniqueness verifies the generic names do not collide
// with the java_* tool set.
func TestGenericToolsJavaNameUniqueness(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range append(CreateCodeAuditTools(), CreateGenericCodeAuditTools()...) {
		assert.Assert(t, !names[tool.Name], "duplicate tool name: %s", tool.Name)
		names[tool.Name] = true
	}
}

// TestGenericProjectProbe_Python verifies language dispatch through probe.
func TestGenericProjectProbe_Python(t *testing.T) {
	tool := findTool(t, CreateGenericCodeAuditTools(), "project_probe")
	dir := genericSampleDir(t, "python")

	result := execTool(t, tool, aitool.InvokeParams{"target": dir, "language": "python"})
	report := parseReport(t, result)
	assert.Equal(t, report["tool"].(string), "codeaudit/probe")

	artifacts, _ := report["artifacts"].(map[string]any)
	assert.Equal(t, artifacts["build_system"].(string), "pip")
}

// TestGenericFrameworkConfigAudit_Multilang verifies config findings across
// python/go/node.
func TestGenericFrameworkConfigAudit_Multilang(t *testing.T) {
	tool := findTool(t, CreateGenericCodeAuditTools(), "framework_config_audit")

	cases := []struct {
		language  string
		framework string
		wantID    string
	}{
		{"python", "django", "py.django.debug"},
		{"go", "go", "go.tls.insecure_skip_verify"},
		{"node", "node", "node.eval.expr"},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			dir := genericSampleDir(t, tc.language)
			result := execTool(t, tool, aitool.InvokeParams{
				"target":    dir,
				"language":  tc.language,
				"framework": tc.framework,
			})
			report := parseReport(t, result)
			assert.Equal(t, report["tool"].(string), "codeaudit/config_audit")

			found := false
			for _, f := range report["findings"].([]any) {
				fm, _ := f.(map[string]any)
				if fm["id"] == tc.wantID {
					found = true
				}
			}
			assert.Assert(t, found, "expected finding %s", tc.wantID)
		})
	}
}

// TestGenericCmsProductAudit_Php verifies the WordPress CMS audit via the
// generic tool.
func TestGenericCmsProductAudit_Php(t *testing.T) {
	tool := findTool(t, CreateGenericCodeAuditTools(), "cms_product_audit")
	dir := genericSampleDir(t, "php")

	result := execTool(t, tool, aitool.InvokeParams{"target": dir, "language": "php"})
	report := parseReport(t, result)
	assert.Equal(t, report["tool"].(string), "codeaudit/cms_audit")

	found := false
	for _, f := range report["findings"].([]any) {
		fm, _ := f.(map[string]any)
		if fm["id"] == "php.wordpress.db_password" {
			found = true
		}
	}
	assert.Assert(t, found, "expected php.wordpress.db_password finding")
}

// TestGenericSecretsScan_Node verifies the generic secrets tool dispatches
// language-agnostic rules.
func TestGenericSecretsScan_Node(t *testing.T) {
	tool := findTool(t, CreateGenericCodeAuditTools(), "secrets_scan")
	dir := genericSampleDir(t, "node")

	result := execTool(t, tool, aitool.InvokeParams{"target": dir, "language": "node"})
	report := parseReport(t, result)
	assert.Equal(t, report["tool"].(string), "codeaudit/secrets")
	assert.Assert(t, len(report["findings"].([]any)) > 0, "expected at least one secret finding")
}

// TestGenericToolsJavaUnchanged verifies the generic tool with language=java
// matches the java_* behavior on the shared spring boot sample.
func TestGenericToolsJavaUnchanged(t *testing.T) {
	tools := CreateGenericCodeAuditTools()
	dir := testDataDir(t, "spring_boot_sample")

	probe := findTool(t, tools, "project_probe")
	result := execTool(t, probe, aitool.InvokeParams{"target": dir, "language": "java"})
	report := parseReport(t, result)
	artifacts, _ := report["artifacts"].(map[string]any)
	assert.Equal(t, artifacts["build_system"].(string), "maven")

	config := findTool(t, tools, "framework_config_audit")
	result = execTool(t, config, aitool.InvokeParams{
		"target": dir, "language": "java", "framework": "spring_boot",
	})
	report = parseReport(t, result)
	found := false
	for _, f := range report["findings"].([]any) {
		fm, _ := f.(map[string]any)
		if fm["id"] == "java.spring.actuator.exposed" {
			found = true
		}
	}
	assert.Assert(t, found, "expected java.spring.actuator.exposed finding for language=java")
}

// TestBuildGenericOptionsDefaults verifies a missing language falls back to
// java without panicking.
func TestBuildGenericOptionsDefaults(t *testing.T) {
	opts := buildGenericOptions(aitool.InvokeParams{})
	assert.Assert(t, len(opts) > 0)
}

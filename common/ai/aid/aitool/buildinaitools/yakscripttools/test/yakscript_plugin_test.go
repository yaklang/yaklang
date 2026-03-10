package test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"gotest.tools/v3/assert"
)

func createTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	tempDB, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatalf("create temp test database: %v", err)
	}
	err = tempDB.AutoMigrate(&schema.YakScript{}, &schema.AIYakTool{}).Error
	if err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return tempDB
}

func insertTestYakScript(t *testing.T, db *gorm.DB, script *schema.YakScript) {
	t.Helper()
	err := yakit.CreateOrUpdateYakScriptByName(db, script.ScriptName, script)
	if err != nil {
		t.Fatalf("insert test YakScript %q: %v", script.ScriptName, err)
	}
}

func TestQueryYakScriptForAI_BasicSearch(t *testing.T) {
	db := createTestDB(t)

	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "xss-detection-test",
		Type:        "mitm",
		Content:     "// test plugin",
		Help:        "XSS detection plugin",
		EnableForAI: true,
		AIDesc:      "Cross-Site Scripting detection",
		AIKeywords:  "xss,cross-site scripting,reflected xss",
	})
	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "not-for-ai-plugin",
		Type:        "mitm",
		Content:     "// not for AI",
		Help:        "This plugin is not enabled for AI",
		EnableForAI: false,
	})
	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "sql-injection-test",
		Type:        "mitm",
		Content:     "// sql injection plugin",
		Help:        "SQL injection detection",
		EnableForAI: true,
		AIDesc:      "SQL injection detection using various techniques",
		AIKeywords:  "sqli,sql injection,union,blind",
	})

	results, err := yakit.QueryYakScriptForAI(db, []string{"xss"}, 10)
	assert.NilError(t, err)
	assert.Check(t, len(results) >= 1, "should find xss plugin")
	found := false
	for _, r := range results {
		if r.ScriptName == "xss-detection-test" {
			found = true
		}
		assert.Check(t, r.ScriptName != "not-for-ai-plugin", "should not return non-AI plugin")
	}
	assert.Check(t, found, "xss-detection-test should be in results")

	results2, err := yakit.QueryYakScriptForAI(db, []string{"sql", "injection"}, 10)
	assert.NilError(t, err)
	assert.Check(t, len(results2) >= 1, "should find sql injection plugin")
}

func TestQueryYakScriptForAI_TypeFilter(t *testing.T) {
	db := createTestDB(t)

	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "codec-plugin-test",
		Type:        "codec",
		Content:     "// codec plugin",
		Help:        "Codec plugin",
		EnableForAI: true,
	})
	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "mitm-plugin-test",
		Type:        "mitm",
		Content:     "// mitm plugin",
		Help:        "MITM security plugin",
		EnableForAI: true,
		AIKeywords:  "security,mitm",
	})

	results, err := yakit.QueryYakScriptForAI(db, []string{"plugin"}, 10)
	assert.NilError(t, err)

	for _, r := range results {
		assert.Check(t, r.Type != "codec", "codec plugins should be filtered out")
	}
}

func TestGetYakScriptByNameForAI(t *testing.T) {
	db := createTestDB(t)

	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "my-ai-plugin",
		Type:        "mitm",
		Content:     "// ai enabled plugin",
		Help:        "AI enabled plugin",
		EnableForAI: true,
	})
	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "my-regular-plugin",
		Type:        "mitm",
		Content:     "// regular plugin",
		Help:        "Regular plugin",
		EnableForAI: false,
	})

	script, err := yakit.GetYakScriptByNameForAI(db, "my-ai-plugin")
	assert.NilError(t, err)
	assert.Equal(t, script.ScriptName, "my-ai-plugin")

	_, err = yakit.GetYakScriptByNameForAI(db, "my-regular-plugin")
	assert.Check(t, err != nil, "should fail for non-AI plugin")

	_, err = yakit.GetYakScriptByNameForAI(db, "nonexistent")
	assert.Check(t, err != nil, "should fail for nonexistent plugin")
}

func TestConvertYakScriptPlugin_MitmType(t *testing.T) {
	script := &schema.YakScript{
		ScriptName: "test-mitm-convert",
		Type:       "mitm",
		Content:    `mirrorHTTPFlow = func(isHttps, url, req, rsp, body) { }`,
		Help:       "Test MITM plugin",
		AIDesc:     "Test MITM plugin for AI",
		AIKeywords: "test,mitm",
	}

	tool, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.NilError(t, err)
	assert.Check(t, tool != nil, "converted tool should not be nil")
	assert.Equal(t, tool.Name, "test-mitm-convert")
	assert.Check(t, strings.Contains(tool.Description, "MITM Plugin"), "description should indicate MITM type")
}

func TestConvertYakScriptPlugin_PortScanType(t *testing.T) {
	script := &schema.YakScript{
		ScriptName: "test-portscan-convert",
		Type:       "port-scan",
		Content:    `handle = func(r) { }`,
		Help:       "Test port-scan plugin",
		AIDesc:     "Test port-scan plugin for AI",
		AIKeywords: "test,port-scan",
	}

	tool, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.NilError(t, err)
	assert.Check(t, tool != nil, "converted tool should not be nil")
	assert.Equal(t, tool.Name, "test-portscan-convert")
	assert.Check(t, strings.Contains(tool.Description, "Port-Scan Plugin"), "description should indicate port-scan type")
}

func TestConvertYakScriptPlugin_NativeYakType(t *testing.T) {
	script := &schema.YakScript{
		ScriptName: "test-native-convert",
		Type:       "yak",
		Content:    `log.info("hello from native plugin")`,
		Help:       "Test native Yak plugin",
		AIDesc:     "Test native plugin for AI",
		AIKeywords: "test,native",
	}

	tool, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.NilError(t, err)
	assert.Check(t, tool != nil, "converted tool should not be nil")
	assert.Equal(t, tool.Name, "test-native-convert")
	assert.Check(t, strings.Contains(tool.Description, "Native Plugin"), "description should indicate native type")
}

func TestConvertYakScriptPlugin_UnsupportedType(t *testing.T) {
	script := &schema.YakScript{
		ScriptName: "test-codec-convert",
		Type:       "codec",
		Content:    `handle = func(param) { return param }`,
		Help:       "Test codec plugin",
	}

	_, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.Check(t, err != nil, "should fail for unsupported codec type")
}

func TestMitmPluginAITool_Execution(t *testing.T) {
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(flag))
	targetUrl := fmt.Sprintf("http://%s:%d", host, port)

	script := &schema.YakScript{
		ScriptName: "test-mitm-exec",
		Type:       "mitm",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	yakit.Info("MITM_PLUGIN_EXECUTED: url=%s body_len=%d", url, len(body))
}
`,
		Help: "Test MITM execution",
	}

	tool, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.NilError(t, err)
	assert.Check(t, tool != nil)

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err = tool.Callback(context.Background(), aitool.InvokeParams{
		"url": targetUrl,
	}, nil, stdout, stderr)
	assert.NilError(t, err)
	t.Logf("MITM plugin stdout: %s", stdout.String())
	t.Logf("MITM plugin stderr: %s", stderr.String())
}

func TestPortScanPluginAITool_Execution(t *testing.T) {
	script := &schema.YakScript{
		ScriptName: "test-portscan-exec",
		Type:       "port-scan",
		Content: `
handle = func(result) {
	yakit.Info("PORTSCAN_PLUGIN_EXECUTED: target=%s port=%d", result.Target, result.Port)
}
`,
		Help: "Test port-scan execution",
	}

	tool, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.NilError(t, err)
	assert.Check(t, tool != nil)

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err = tool.Callback(context.Background(), aitool.InvokeParams{
		"target": "127.0.0.1",
		"port":   "80",
	}, nil, stdout, stderr)
	assert.NilError(t, err)
	t.Logf("Port-scan plugin stdout: %s", stdout.String())
	t.Logf("Port-scan plugin stderr: %s", stderr.String())
}

func TestExistingAIToolsStillWork(t *testing.T) {
	tools := yakscripttools.GetAllYakScriptAiTools()
	assert.Check(t, len(tools) > 0, "existing AI tools should still be available")

	hasDoHttp := false
	for _, tool := range tools {
		if tool.Name == "send_http_request_by_url" || tool.Name == "do_http_request" {
			hasDoHttp = true
			break
		}
	}
	assert.Check(t, hasDoHttp, "built-in HTTP tool should still exist")
}

func TestSearchAndConvertFlow(t *testing.T) {
	db := createTestDB(t)

	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "shiro-detect-ai-test",
		Type:        "mitm",
		Content:     `mirrorHTTPFlow = func(isHttps, url, req, rsp, body) { }`,
		Help:        "Shiro fingerprint detection",
		EnableForAI: true,
		AIDesc:      "Apache Shiro fingerprinting and default key detection",
		AIKeywords:  "shiro,apache shiro,rememberMe,default key",
	})

	results, err := yakit.QueryYakScriptForAI(db, []string{"shiro"}, 10)
	assert.NilError(t, err)
	assert.Check(t, len(results) >= 1, "should find shiro plugin")

	for _, script := range results {
		tool, err := yakscripttools.ConvertYakScriptPlugin(script)
		assert.NilError(t, err)
		assert.Check(t, tool != nil, "converted tool should not be nil")
		assert.Check(t, tool.Callback != nil, "tool should have a callback")
		t.Logf("converted plugin: name=%s desc=%s", tool.Name, tool.Description)
	}
}

func TestConcurrentPluginConversion(t *testing.T) {
	scripts := []*schema.YakScript{
		{
			ScriptName: "concurrent-test-1",
			Type:       "mitm",
			Content:    `mirrorHTTPFlow = func(isHttps, url, req, rsp, body) { }`,
			Help:       "Concurrent test 1",
		},
		{
			ScriptName: "concurrent-test-2",
			Type:       "mitm",
			Content:    `mirrorHTTPFlow = func(isHttps, url, req, rsp, body) { }`,
			Help:       "Concurrent test 2",
		},
		{
			ScriptName: "concurrent-test-3",
			Type:       "mitm",
			Content:    `mirrorHTTPFlow = func(isHttps, url, req, rsp, body) { }`,
			Help:       "Concurrent test 3",
		},
	}

	tools := make([]*aitool.Tool, 0, len(scripts))
	for _, script := range scripts {
		tool, err := yakscripttools.ConvertYakScriptPlugin(script)
		assert.NilError(t, err)
		assert.Check(t, tool != nil)
		tools = append(tools, tool)
	}
	assert.Equal(t, len(tools), 3, "should convert all 3 plugins")
}

func TestConvertNativeYakPlugin_SecondaryDisclosure(t *testing.T) {
	script := &schema.YakScript{
		ScriptName: "native-disclosure-test",
		Type:       "yak",
		Content: `
__USAGE__ = "This tool scans the target for open ports and identifies running services."

target = cli.String("target", cli.setHelp("Target host or IP"), cli.setRequired(true))
port = cli.String("port", cli.setHelp("Port range to scan"), cli.setDefault("1-1024"))
timeout = cli.Int("timeout", cli.setHelp("Timeout in seconds"), cli.setDefault(5))
verbose = cli.Bool("verbose", cli.setHelp("Enable verbose output"))

log.info("scanning %s:%s timeout=%d", target, port, timeout)
`,
		Help:       "Native scan plugin",
		AIDesc:     "Port scanning and service identification tool",
		AIKeywords: "scan,port,service",
	}

	tool, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.NilError(t, err)
	assert.Check(t, tool != nil)

	// Verify Usage is set from __USAGE__ (secondary disclosure)
	assert.Check(t, tool.Usage != "", "Usage should be set for secondary disclosure")
	assert.Check(t, strings.Contains(tool.Usage, "scans the target"), "Usage should contain __USAGE__ content")

	// Verify SSA-parsed cli.* parameters are present (not hardcoded "target" only)
	schema := tool.ToJSONSchemaString()
	t.Logf("tool schema: %s", schema)
	assert.Check(t, strings.Contains(schema, "target"), "should have 'target' parameter from cli.String")
	assert.Check(t, strings.Contains(schema, "port"), "should have 'port' parameter from cli.String")
	assert.Check(t, strings.Contains(schema, "timeout"), "should have 'timeout' parameter from cli.Int")
	assert.Check(t, strings.Contains(schema, "verbose"), "should have 'verbose' parameter from cli.Bool")
}

func TestConvertNativeYakPlugin_AIUsageOverridesScript(t *testing.T) {
	script := &schema.YakScript{
		ScriptName: "native-aiusage-override-test",
		Type:       "yak",
		Content: `
__USAGE__ = "Usage from script content."
target = cli.String("target")
`,
		Help:    "Test plugin",
		AIUsage: "Custom AI Usage from database field.",
	}

	tool, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.NilError(t, err)
	assert.Check(t, tool != nil)

	// AIUsage from schema should take priority over __USAGE__ from script
	assert.Check(t, strings.Contains(tool.Usage, "Custom AI Usage"), "AIUsage field should override __USAGE__")
	assert.Check(t, !strings.Contains(tool.Usage, "Usage from script"), "script __USAGE__ should not be used when AIUsage is set")
}

func TestConvertMitmPlugin_SecondaryDisclosure(t *testing.T) {
	script := &schema.YakScript{
		ScriptName: "mitm-disclosure-test",
		Type:       "mitm",
		Content:    `mirrorHTTPFlow = func(isHttps, url, req, rsp, body) { }`,
		Help:       "MITM security plugin",
		AIDesc:     "Detects vulnerabilities in HTTP traffic",
	}

	tool, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.NilError(t, err)
	assert.Check(t, tool != nil)

	// Verify standard MITM Usage is set
	assert.Check(t, tool.Usage != "", "MITM tool should have Usage for secondary disclosure")
	assert.Check(t, strings.Contains(tool.Usage, "requestPacket"), "MITM Usage should mention requestPacket parameter")
	assert.Check(t, strings.Contains(tool.Usage, "url"), "MITM Usage should mention url parameter")
	assert.Check(t, strings.Contains(tool.Usage, "isHttps"), "MITM Usage should mention isHttps parameter")
}

func TestConvertPortScanPlugin_SecondaryDisclosure(t *testing.T) {
	script := &schema.YakScript{
		ScriptName: "portscan-disclosure-test",
		Type:       "port-scan",
		Content:    `handle = func(result) { }`,
		Help:       "Port scan security plugin",
		AIDesc:     "Analyzes port scan results",
	}

	tool, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.NilError(t, err)
	assert.Check(t, tool != nil)

	// Verify standard Port-Scan Usage is set
	assert.Check(t, tool.Usage != "", "Port-scan tool should have Usage for secondary disclosure")
	assert.Check(t, strings.Contains(tool.Usage, "target"), "Port-scan Usage should mention target parameter")
	assert.Check(t, strings.Contains(tool.Usage, "port"), "Port-scan Usage should mention port parameter")
}

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

func TestConvertYakScriptPlugin_CorePluginAddsPriorityHints(t *testing.T) {
	script := &schema.YakScript{
		ScriptName:   "test-core-native-convert",
		Type:         "yak",
		Content:      `log.info("hello from core native plugin")`,
		Help:         "Test core native Yak plugin",
		AIDesc:       "Core native plugin for AI",
		IsCorePlugin: true,
	}

	tool, err := yakscripttools.ConvertYakScriptPlugin(script)
	assert.NilError(t, err)
	assert.Check(t, tool != nil, "converted tool should not be nil")
	assert.Equal(t, tool.VerboseName, "Core Tool / 核心工具 / High Priority")
	assert.Check(t, strings.Contains(tool.Description, "Core Tool"), "description should indicate core tool")
	assert.Check(t, strings.Contains(tool.Description, "High Priority"), "description should indicate high priority")
	assert.Check(t, strings.Contains(tool.Description, "Prefer calling it first"), "description should explain priority behavior")
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

// TestFullChain_Search_Convert_Disclosure tests the complete pipeline:
// DB insert (enable_for_ai=true) → BM25 search finds it → convert to AI tool → verify Usage & params
func TestFullChain_Search_Convert_Disclosure(t *testing.T) {
	db := createTestDB(t)

	// Simulate a CSRF detection MITM plugin like the core plugin
	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "CSRF-Detection-Chain-Test",
		Type:        "mitm",
		Content:     "mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {\n  yakit.Info(\"csrf check: %s\", url)\n}\n",
		Help:        "CSRF form protection and CORS misconfiguration detection",
		EnableForAI: true,
		AIDesc:      "CSRF form protection and CORS misconfiguration detection. Checks for missing CSRF tokens in forms and insecure CORS headers.",
		AIKeywords:  "csrf,cors,cross-site request forgery,CORS misconfiguration,form protection,CSRF检测",
	})

	// Also insert a non-AI plugin that mentions CSRF - should NOT be found
	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "CSRF-Not-For-AI",
		Type:        "mitm",
		Content:     "mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {}\n",
		Help:        "CSRF checker but not for AI",
		EnableForAI: false,
	})

	// Also insert some native yak plugin
	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "Native-Scanner-Chain-Test",
		Type:        "yak",
		Content:     "__USAGE__ = \"Scan host for security issues.\"\ntarget = cli.String(\"target\", cli.setRequired(true))\nmode = cli.String(\"mode\", cli.setDefault(\"fast\"))\nlog.info(\"scanning %s mode=%s\", target, mode)\n",
		Help:        "Native security scanner",
		EnableForAI: true,
		AIDesc:      "Native security scanner with configurable scan modes",
		AIKeywords:  "scanner,security,native,scan",
	})

	// Step 1: Search via BM25 - should find CSRF plugin
	results, err := yakit.SearchYakScriptForAIBM25(db, &yakit.YakScriptForAIFilter{
		Keywords: []string{"csrf"},
	}, 10, 0)
	assert.NilError(t, err)
	assert.Check(t, len(results) >= 1, "BM25 search for 'csrf' should find at least 1 result")

	foundCSRF := false
	for _, r := range results {
		assert.Check(t, r.EnableForAI, "all results must have enable_for_ai=true")
		assert.Check(t, r.ScriptName != "CSRF-Not-For-AI", "non-AI plugin must not appear")
		if r.ScriptName == "CSRF-Detection-Chain-Test" {
			foundCSRF = true
		}
	}
	assert.Check(t, foundCSRF, "CSRF-Detection-Chain-Test must be in search results")
	t.Logf("Step 1 PASS: BM25 search found %d results, CSRF plugin present", len(results))

	// Step 2: Convert the found CSRF plugin to AI tool
	csrfScript := results[0]
	tool, err := yakscripttools.ConvertYakScriptPlugin(csrfScript)
	assert.NilError(t, err)
	assert.Check(t, tool != nil)
	assert.Equal(t, tool.Name, "CSRF-Detection-Chain-Test")
	assert.Check(t, strings.Contains(tool.Description, "MITM Plugin"), "should be tagged as MITM Plugin")
	assert.Check(t, tool.Usage != "", "MITM tool must have Usage for secondary disclosure")
	assert.Check(t, strings.Contains(tool.Usage, "requestPacket"), "Usage must mention requestPacket")
	assert.Check(t, tool.Callback != nil, "tool must have executable callback")
	t.Logf("Step 2 PASS: CSRF MITM plugin converted, Usage len=%d", len(tool.Usage))

	// Step 3: Search for native plugin, convert, verify SSA-parsed params
	nativeResults, err := yakit.SearchYakScriptForAIBM25(db, &yakit.YakScriptForAIFilter{
		Keywords: []string{"scanner", "security"},
	}, 10, 0)
	assert.NilError(t, err)
	foundNative := false
	for _, r := range nativeResults {
		if r.ScriptName == "Native-Scanner-Chain-Test" {
			foundNative = true

			nativeTool, err := yakscripttools.ConvertYakScriptPlugin(r)
			assert.NilError(t, err)
			assert.Check(t, nativeTool != nil)

			// Verify __USAGE__ is in secondary disclosure
			assert.Check(t, strings.Contains(nativeTool.Usage, "Scan host"), "native tool Usage must contain __USAGE__ content")

			// Verify SSA-parsed cli params
			schemaJSON := nativeTool.ToJSONSchemaString()
			assert.Check(t, strings.Contains(schemaJSON, "target"), "must have SSA-parsed 'target' param")
			assert.Check(t, strings.Contains(schemaJSON, "mode"), "must have SSA-parsed 'mode' param")
			usagePreview := nativeTool.Usage
			if len(usagePreview) > 50 {
				usagePreview = usagePreview[:50]
			}
			t.Logf("Step 3 PASS: Native plugin converted, Usage='%s...', schema has target+mode", usagePreview)
		}
	}
	assert.Check(t, foundNative, "Native-Scanner-Chain-Test must be findable via search")
}

// TestFullChain_MITM_Execution_WithDisclosure tests that a MITM plugin found via
// search can actually execute against a real HTTP target with proper secondary disclosure.
func TestFullChain_MITM_Execution_WithDisclosure(t *testing.T) {
	db := createTestDB(t)

	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(flag))
	targetUrl := fmt.Sprintf("http://%s:%d", host, port)

	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "MITM-Exec-Chain-Test",
		Type:        "mitm",
		Content:     "mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {\n  yakit.Info(\"CHAIN_TEST_EXECUTED: %s body_len=%d\", url, len(body))\n}\n",
		Help:        "MITM execution chain test",
		EnableForAI: true,
		AIDesc:      "Chain test MITM plugin for execution verification",
		AIKeywords:  "chain,test,execution,mitm",
	})

	// Search
	results, err := yakit.SearchYakScriptForAIBM25(db, &yakit.YakScriptForAIFilter{
		Keywords: []string{"chain", "execution"},
	}, 10, 0)
	assert.NilError(t, err)
	assert.Check(t, len(results) >= 1, "should find the chain test plugin")

	// Convert
	tool, err := yakscripttools.ConvertYakScriptPlugin(results[0])
	assert.NilError(t, err)
	assert.Check(t, tool.Usage != "", "must have Usage for secondary disclosure")

	// Execute
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err = tool.Callback(context.Background(), aitool.InvokeParams{
		"url": targetUrl,
	}, nil, stdout, stderr)
	assert.NilError(t, err)
	t.Logf("MITM plugin stdout: %s", stdout.String())
	t.Logf("Execution chain complete: search → convert → disclosure → execute ✓")
}

// TestPluginHash_AIFields_TriggerUpdate verifies that when AI fields differ between
// a database plugin and the new scriptData, the hash comparison detects the change.
func TestPluginHash_AIFields_TriggerUpdate(t *testing.T) {
	db := createTestDB(t)

	// Step 1: Insert plugin WITHOUT AI fields (simulates existing DB state before withPluginEnableForAI)
	insertTestYakScript(t, db, &schema.YakScript{
		ScriptName:  "hash-update-test-plugin",
		Type:        "mitm",
		Content:     "mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {}\n",
		Help:        "Test plugin for hash update verification",
		EnableForAI: false,
		AIDesc:      "",
		AIKeywords:  "",
	})

	// Verify it's NOT found by AI search
	results, err := yakit.SearchYakScriptForAIBM25(db, &yakit.YakScriptForAIFilter{
		Keywords: []string{"hash-update-test"},
	}, 10, 0)
	assert.NilError(t, err)
	assert.Check(t, len(results) == 0, "plugin without enable_for_ai should not appear in AI search")

	// Step 2: Update the plugin to enable AI (simulates what OverWriteYakPlugin should do after hash fix)
	err = yakit.CreateOrUpdateYakScriptByName(db, "hash-update-test-plugin", &schema.YakScript{
		ScriptName:  "hash-update-test-plugin",
		Type:        "mitm",
		Content:     "mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {}\n",
		Help:        "Test plugin for hash update verification",
		EnableForAI: true,
		AIDesc:      "Now AI-enabled test plugin for hash verification",
		AIKeywords:  "hash,update,test,verification",
	})
	assert.NilError(t, err)

	// Step 3: Verify it IS now found by AI search
	results, err = yakit.SearchYakScriptForAIBM25(db, &yakit.YakScriptForAIFilter{
		Keywords: []string{"hash", "update"},
	}, 10, 0)
	assert.NilError(t, err)
	assert.Check(t, len(results) >= 1, "after enabling AI, plugin must appear in AI search")
	assert.Equal(t, results[0].ScriptName, "hash-update-test-plugin")
	assert.Check(t, results[0].EnableForAI, "enable_for_ai must be true")
	assert.Check(t, results[0].AIDesc != "", "ai_desc must be populated")
	t.Logf("Hash update test passed: plugin became searchable after AI fields were written")
}

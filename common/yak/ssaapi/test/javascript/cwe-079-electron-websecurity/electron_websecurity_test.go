package cwe079electronwebsecurity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// loadInsecureContentRule 从内置 embed FS 读取 js-enabling-electron-insecure-content.sf 规则内容。
func loadInsecureContentRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-079-electron-websecurity/js-enabling-electron-insecure-content.sf")
	if !ok {
		t.Skip("js-enabling-electron-insecure-content.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "js-enabling-electron-insecure-content.sf 内容为空")
	return content
}

// loadWebSecurityRule 从内置 embed FS 读取 js-disabling-electron-websecurity.sf 规则内容。
func loadWebSecurityRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-079-electron-websecurity/js-disabling-electron-websecurity.sf")
	if !ok {
		t.Skip("js-disabling-electron-websecurity.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "js-disabling-electron-websecurity.sf 内容为空")
	return content
}

// runOnCode 用单文件 VirtualFS 执行规则，返回告警总数。
func runOnCode(t *testing.T, ruleContent, filename, code string) int {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, code)

	total := 0
	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		require.Greater(t, len(programs), 0, "SSA 编译应至少产生一个程序")
		result, err := programs[0].SyntaxFlowWithError(ruleContent)
		require.NoError(t, err, "规则执行不应报错")
		for _, varName := range result.GetAlertVariables() {
			total += len(result.GetValues(varName))
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JS))

	return total
}

// TestWebSecurity_Positive_BasicFalse 基础检测：webSecurity: false
// 对应 CodeQL 文档中的标准漏洞示例。
func TestWebSecurity_Positive_BasicFalse(t *testing.T) {
	rule := loadWebSecurityRule(t)
	code := `
const { BrowserWindow } = require('electron');

const mainWindow = new BrowserWindow({
    webPreferences: {
        webSecurity: false
    }
});
mainWindow.loadURL('https://example.com');
`
	total := runOnCode(t, rule, "unsafe_basic.js", code)
	assert.Greater(t, total, 0, "webSecurity: false 应触发告警")
}

// TestWebSecurity_Positive_WithNodeIntegration 检测：同时开启 nodeIntegration 和禁用 webSecurity（高风险组合）
func TestWebSecurity_Positive_WithNodeIntegration(t *testing.T) {
	rule := loadWebSecurityRule(t)
	code := `
const { BrowserWindow } = require('electron');

const win = new BrowserWindow({
    width: 800,
    height: 600,
    webPreferences: {
        nodeIntegration: true,
        webSecurity: false,
        contextIsolation: false
    }
});
win.loadURL('https://attacker.example.com');
`
	total := runOnCode(t, rule, "unsafe_node_integration.js", code)
	assert.Greater(t, total, 0, "nodeIntegration+webSecurity:false 应触发告警")
}

// TestWebSecurity_Positive_ViaVariable 通过变量间接赋值也应检测到
func TestWebSecurity_Positive_ViaVariable(t *testing.T) {
	rule := loadWebSecurityRule(t)
	code := `
const { BrowserWindow } = require('electron');

const webPrefs = {
    webSecurity: false
};

const win = new BrowserWindow({
    webPreferences: webPrefs
});
`
	total := runOnCode(t, rule, "unsafe_via_var.js", code)
	assert.Greater(t, total, 0, "通过变量传递 webSecurity:false 应触发告警")
}

// TestWebSecurity_Negative_ExplicitTrue 显式设置 webSecurity: true（安全）
func TestWebSecurity_Negative_ExplicitTrue(t *testing.T) {
	rule := loadWebSecurityRule(t)
	code := `
const { BrowserWindow } = require('electron');

const mainWindow = new BrowserWindow({
    webPreferences: {
        webSecurity: true,
        contextIsolation: true,
        nodeIntegration: false
    }
});
`
	total := runOnCode(t, rule, "safe_explicit_true.js", code)
	assert.Equal(t, 0, total, "webSecurity: true 不应触发告警")
}

// TestWebSecurity_Negative_DefaultValue 未设置 webSecurity（默认为 true，安全）
func TestWebSecurity_Negative_DefaultValue(t *testing.T) {
	rule := loadWebSecurityRule(t)
	code := `
const { BrowserWindow } = require('electron');

// GOOD: webSecurity defaults to true when not specified
const mainWindow = new BrowserWindow({
    width: 1200,
    height: 800,
    webPreferences: {
        contextIsolation: true,
        nodeIntegration: false,
        preload: './preload.js'
    }
});
`
	total := runOnCode(t, rule, "safe_default.js", code)
	assert.Equal(t, 0, total, "未设置 webSecurity 时不应触发告警")
}

// TestWebSecurity_Negative_NoWebPreferences 未设置 webPreferences
func TestWebSecurity_Negative_NoWebPreferences(t *testing.T) {
	rule := loadWebSecurityRule(t)
	code := `
const { BrowserWindow } = require('electron');

const mainWindow = new BrowserWindow({
    width: 800,
    height: 600
});
`
	total := runOnCode(t, rule, "safe_no_webprefs.js", code)
	assert.Equal(t, 0, total, "未设置 webPreferences 时不应触发告警")
}

// ============================================================
// js-enabling-electron-insecure-content 规则测试
// ============================================================

// TestInsecureContent_Positive_BasicTrue 基础检测：allowRunningInsecureContent: true
// 对应 CodeQL 文档中的标准漏洞示例。
func TestInsecureContent_Positive_BasicTrue(t *testing.T) {
	rule := loadInsecureContentRule(t)
	code := `
const { BrowserWindow } = require('electron');

const mainWindow = new BrowserWindow({
    webPreferences: {
        allowRunningInsecureContent: true
    }
});
mainWindow.loadURL('https://example.com');
`
	total := runOnCode(t, rule, "unsafe_insecure_content.js", code)
	assert.Greater(t, total, 0, "allowRunningInsecureContent: true 应触发告警")
}

// TestInsecureContent_Positive_WithNodeIntegration 高危组合：nodeIntegration + allowRunningInsecureContent
func TestInsecureContent_Positive_WithNodeIntegration(t *testing.T) {
	rule := loadInsecureContentRule(t)
	code := `
const { BrowserWindow } = require('electron');

const win = new BrowserWindow({
    width: 1024,
    height: 768,
    webPreferences: {
        nodeIntegration: true,
        contextIsolation: false,
        allowRunningInsecureContent: true
    }
});
`
	total := runOnCode(t, rule, "unsafe_insecure_content_node.js", code)
	assert.Greater(t, total, 0, "nodeIntegration + allowRunningInsecureContent:true 应触发告警")
}

// TestInsecureContent_Negative_ExplicitFalse 显式设置 allowRunningInsecureContent: false（安全）
func TestInsecureContent_Negative_ExplicitFalse(t *testing.T) {
	rule := loadInsecureContentRule(t)
	code := `
const { BrowserWindow } = require('electron');

const mainWindow = new BrowserWindow({
    webPreferences: {
        allowRunningInsecureContent: false,
        contextIsolation: true
    }
});
`
	total := runOnCode(t, rule, "safe_insecure_content_false.js", code)
	assert.Equal(t, 0, total, "allowRunningInsecureContent: false 不应触发告警")
}

// TestInsecureContent_Negative_DefaultValue 未设置该属性（默认 false，安全）
func TestInsecureContent_Negative_DefaultValue(t *testing.T) {
	rule := loadInsecureContentRule(t)
	code := `
const { BrowserWindow } = require('electron');

// GOOD: allowRunningInsecureContent defaults to false
const mainWindow = new BrowserWindow({
    width: 800,
    height: 600,
    webPreferences: {
        contextIsolation: true,
        nodeIntegration: false
    }
});
`
	total := runOnCode(t, rule, "safe_insecure_content_default.js", code)
	assert.Equal(t, 0, total, "未设置 allowRunningInsecureContent 时不应触发告警")
}

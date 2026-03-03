package cwe614sensitivecookie

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

// loadClearTextCookieRule 从内置 embed FS 读取 js-clear-text-cookie.sf 规则内容。
func loadClearTextCookieRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-614-sensitive-cookie-without-secure-flag/js-clear-text-cookie.sf")
	if !ok {
		t.Skip("ecmascript/cwe-614-sensitive-cookie-without-secure-flag/js-clear-text-cookie.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "js-clear-text-cookie.sf 内容为空")
	return content
}

// runOnFile 用单文件 VirtualFS 执行规则，返回 (totalAlerts, warningAlerts)。
func runOnFile(t *testing.T, ruleContent, filename, code string) (total, warning int) {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, code)

	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		require.Greater(t, len(programs), 0, "SSA 编译应至少产生一个程序")
		result, err := programs[0].SyntaxFlowWithError(ruleContent)
		require.NoError(t, err, "规则执行不应报错")
		for _, varName := range result.GetAlertVariables() {
			vals := result.GetValues(varName)
			total += len(vals)
			if info, ok := result.GetAlertInfo(varName); ok {
				if info.Severity == "warning" || info.Severity == "warn" || info.Severity == "w" {
					warning += len(vals)
				}
			}
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JS))
	return
}

// ============================================================
// Pattern A: res.setHeader("Set-Cookie", ...)
// ============================================================

// TestClearTextCookie_Positive_SetHeaderTemplate 验证 setHeader 使用不含 Secure 的模板字符串应触发告警。
// 对应 irify/ts-sf-rules/cwe-614/clear-text-transmission-of-sensitive-cookie/positive.js
func TestClearTextCookie_Positive_SetHeaderTemplate(t *testing.T) {
	rule := loadClearTextCookieRule(t)
	total, _ := runOnFile(t, rule, "positive.js", `
const http = require('http');

const server = http.createServer((req, res) => {
    res.setHeader("Set-Cookie", `+"`"+`authKey=${makeAuthkey()}`+"`"+`);
    res.writeHead(200, { 'Content-Type': 'text/html' });
    res.end('<h2>Hello world</h2>');
});
`)
	assert.Greater(t, total, 0, "不含 Secure 的模板字符串 Cookie 应触发告警（漏报）")
}

// TestClearTextCookie_Negative_SetHeaderTemplateWithSecure 验证 setHeader 含 secure; httpOnly 的模板字符串不触发告警。
// 对应 irify/ts-sf-rules/cwe-614/clear-text-transmission-of-sensitive-cookie/negative.js
func TestClearTextCookie_Negative_SetHeaderTemplateWithSecure(t *testing.T) {
	rule := loadClearTextCookieRule(t)
	total, _ := runOnFile(t, rule, "negative.js", `
const http = require('http');

const server = http.createServer((req, res) => {
    res.setHeader("Set-Cookie", `+"`"+`authKey=${makeAuthkey()}; secure; httpOnly`+"`"+`);
    res.writeHead(200, { 'Content-Type': 'text/html' });
    res.end('<h2>Hello world</h2>');
});
`)
	assert.Equal(t, 0, total, "含 Secure 属性的 Cookie 不应触发告警（误报）")
}

// TestClearTextCookie_Positive_SetHeaderStaticNoSecure 验证静态字符串 Cookie 缺少 Secure 属性时触发告警。
func TestClearTextCookie_Positive_SetHeaderStaticNoSecure(t *testing.T) {
	rule := loadClearTextCookieRule(t)
	total, _ := runOnFile(t, rule, "static_no_secure.js", `
const http = require('http');

const server = http.createServer((req, res) => {
    res.setHeader("Set-Cookie", "sessionToken=abc123; HttpOnly");
    res.end('OK');
});
`)
	assert.Greater(t, total, 0, "静态 Cookie 字符串缺少 Secure 应触发告警（漏报）")
}

// TestClearTextCookie_Negative_SetHeaderStaticWithSecure 验证静态字符串 Cookie 包含 Secure 属性时不触发告警。
func TestClearTextCookie_Negative_SetHeaderStaticWithSecure(t *testing.T) {
	rule := loadClearTextCookieRule(t)
	total, _ := runOnFile(t, rule, "static_with_secure.js", `
const http = require('http');

const server = http.createServer((req, res) => {
    res.setHeader("Set-Cookie", "sessionToken=abc123; Secure; HttpOnly");
    res.end('OK');
});
`)
	assert.Equal(t, 0, total, "静态 Cookie 字符串包含 Secure 不应触发告警（误报）")
}

// TestClearTextCookie_Positive_SetHeaderTemplateHttpOnlyNoSecure 验证有 httpOnly 但无 Secure 时仍触发告警。
func TestClearTextCookie_Positive_SetHeaderTemplateHttpOnlyNoSecure(t *testing.T) {
	rule := loadClearTextCookieRule(t)
	total, _ := runOnFile(t, rule, "template_httponly_no_secure.js", `
const http = require('http');
function makeAuthkey() { return "key123"; }

const server = http.createServer((req, res) => {
    res.setHeader("Set-Cookie", `+"`"+`authKey=${makeAuthkey()}; httpOnly`+"`"+`);
    res.end('OK');
});
`)
	assert.Greater(t, total, 0, "有 httpOnly 但无 Secure 仍应触发告警（漏报）")
}

// TestClearTextCookie_Negative_SetHeaderTemplateMixedCaseSecure 验证大小写混合 Secure 关键字时不误报。
func TestClearTextCookie_Negative_SetHeaderTemplateMixedCaseSecure(t *testing.T) {
	rule := loadClearTextCookieRule(t)
	total, _ := runOnFile(t, rule, "template_mixed_case_secure.js", `
const http = require('http');
function makeAuthkey() { return "key123"; }

const server = http.createServer((req, res) => {
    res.setHeader("Set-Cookie", `+"`"+`token=${makeAuthkey()}; Secure; HttpOnly`+"`"+`);
    res.end('OK');
});
`)
	assert.Equal(t, 0, total, "大小写混合 Secure 不应触发告警（误报）")
}

// ============================================================
// Pattern B: Express res.cookie(..., { secure: false })
// ============================================================

// TestClearTextCookie_Positive_ExpressSecureFalse 验证 Express res.cookie() 明确设置 secure: false 时触发告警。
func TestClearTextCookie_Positive_ExpressSecureFalse(t *testing.T) {
	rule := loadClearTextCookieRule(t)
	total, _ := runOnFile(t, rule, "express_secure_false.js", `
const express = require('express');
const app = express();

app.post('/login', (req, res) => {
    const token = generateToken(req.body.username);
    res.cookie('authToken', token, {
        httpOnly: true,
        secure: false
    });
    res.json({ success: true });
});
`)
	assert.Greater(t, total, 0, "secure: false 应触发告警（漏报）")
}

// TestClearTextCookie_Negative_ExpressSecureTrue 验证 Express res.cookie() 设置 secure: true 时不触发告警。
func TestClearTextCookie_Negative_ExpressSecureTrue(t *testing.T) {
	rule := loadClearTextCookieRule(t)
	total, _ := runOnFile(t, rule, "express_secure_true.js", `
const express = require('express');
const app = express();

app.post('/login', (req, res) => {
    const token = generateToken(req.body.username);
    res.cookie('authToken', token, {
        httpOnly: true,
        secure: true,
        sameSite: 'Strict'
    });
    res.json({ success: true });
});
`)
	assert.Equal(t, 0, total, "secure: true 不应触发告警（误报）")
}

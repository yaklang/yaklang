package cwe601urlredirection

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

func loadRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-601-url-redirection/js-server-side-unvalidated-url-redirection.sf")
	if !ok {
		t.Skip("js-server-side-unvalidated-url-redirection.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content)
	return content
}

func runOnFile(t *testing.T, rule, filename, code string) int {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, code)
	total := 0
	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		require.Greater(t, len(programs), 0)
		result, err := programs[0].SyntaxFlowWithError(rule)
		require.NoError(t, err)
		for _, v := range result.GetAlertVariables() {
			total += len(result.GetValues(v))
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JS))
	return total
}

// TestURLRedirection_Positive 验证直接将请求参数用于重定向触发告警。
func TestURLRedirection_Positive(t *testing.T) {
	rule := loadRule(t)
	code := `
const app = require("express")();

app.get("/redirect", function (req, res) {
    // BAD: a request parameter is incorporated without validation into a URL redirect
    res.redirect(req.query["target"]);
});
`
	total := runOnFile(t, rule, "positive.js", code)
	assert.Greater(t, total, 0, "直接重定向用户输入应触发告警（漏报）")
}

// TestURLRedirection_NegativeExactConstant 验证等值常量校验后不触发告警。
func TestURLRedirection_NegativeExactConstant(t *testing.T) {
	rule := loadRule(t)
	code := `
const app = require("express")();

const VALID_REDIRECT = "http://cwe.mitre.org/data/definitions/601.html";

app.get("/redirect", function (req, res) {
    // GOOD: the request parameter is validated against a known fixed string
    let target = req.query["target"];
    if (VALID_REDIRECT === target) {
        res.redirect(target);
    } else {
        res.redirect("/");
    }
});
`
	total := runOnFile(t, rule, "negative.js", code)
	assert.Equal(t, 0, total, "等值常量校验后不应触发告警（误报）")
}

// TestURLRedirection_NegativeAllowlist 验证服务端 allowlist 映射不触发告警。
func TestURLRedirection_NegativeAllowlist(t *testing.T) {
	rule := loadRule(t)
	code := `
const app = require("express")();

app.get("/redirect", function (req, res) {
    const dest = req.query.dest;
    let url;
    if (dest === "home") {
        url = "/home";
    } else {
        url = "/default";
    }
    // GOOD: url is phi of constants, not user-controlled
    res.redirect(url);
});
`
	total := runOnFile(t, rule, "negative_allowlist.js", code)
	assert.Equal(t, 0, total, "allowlist 映射（phi of constants）不应触发告警（误报）")
}

// TestURLRedirection_ConcatPositive 验证字符串拼接后重定向触发告警。
func TestURLRedirection_ConcatPositive(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require("express");
const app = express();

app.get("/go", (req, res) => {
    const dest = req.query.url;
    // BAD: user input appended to base URL
    res.redirect("https://example.com/" + dest);
});
`
	total := runOnFile(t, rule, "concat.js", code)
	assert.Greater(t, total, 0, "字符串拼接重定向应触发告警")
}

// TestURLRedirection_BodyParamPositive 验证 POST body 参数触发告警。
func TestURLRedirection_BodyParamPositive(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require("express");
const app = express();

app.post("/login", (req, res) => {
    // BAD: redirect target from POST body
    res.redirect(req.body.next);
});
`
	total := runOnFile(t, rule, "body.js", code)
	assert.Greater(t, total, 0, "POST body 参数重定向应触发告警")
}

// TestURLRedirection_PartialGuardPositive 验证部分守卫（else 分支仍含用户输入）触发告警。
func TestURLRedirection_PartialGuardPositive(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require("express");
const app = express();

app.get("/redirect", (req, res) => {
    const target = req.query.target;
    let url;
    if (target === "safe") {
        url = "/safe";
    } else {
        // BAD: else branch still returns attacker-controlled value
        url = target;
    }
    res.redirect(url);
});
`
	total := runOnFile(t, rule, "partial_guard.js", code)
	assert.Greater(t, total, 0, "部分守卫（else 分支含用户输入）应触发告警")
}

// TestURLRedirection_HardcodedNoAlert 验证硬编码目标不触发告警。
func TestURLRedirection_HardcodedNoAlert(t *testing.T) {
	rule := loadRule(t)
	code := `
const app = require("express")();

app.get("/logout", (req, res) => {
    // GOOD: redirect target is hardcoded
    res.redirect("/login");
});
`
	total := runOnFile(t, rule, "hardcoded.js", code)
	assert.Equal(t, 0, total, "硬编码重定向目标不应触发告警（误报）")
}

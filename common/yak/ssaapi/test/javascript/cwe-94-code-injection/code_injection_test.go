package cwe94codeinjection

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

// loadCodeInjectionRule 从内置 embed FS 读取 js-code-injection.sf 规则内容。
func loadCodeInjectionRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-094-code-injection/js-code-injection.sf")
	if !ok {
		t.Skip("ecmascript/cwe-094-code-injection/js-code-injection.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "js-code-injection.sf 内容为空")
	return content
}

// runOnFile 用单文件 VirtualFS 执行规则，返回 (totalAlerts, highAlerts)。
func runOnFile(t *testing.T, ruleContent, filename, code string) (total, high int) {
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
				if info.Severity == "high" || info.Severity == "h" {
					high += len(vals)
				}
			}
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JS))
	return
}

// ============================================================
// Positive: 直接 eval 注入（HIGH）
// ============================================================

// TestCodeInject_Positive_EvalDocumentLocation 验证 eval(document.location...) 浏览器端 URL 注入触发告警。
// 对应 irify/ts-sf-rules/cwe-94/code-inject/positive.js
// 注意：document.location.href.substring(...) 经过了 .substring() 调用，规则分类为 mid（间接注入）而非 high。
func TestCodeInject_Positive_EvalDocumentLocation(t *testing.T) {
	rule := loadCodeInjectionRule(t)
	total, _ := runOnFile(t, rule, "positive.js", `
eval(document.location.href.substring(document.location.href.indexOf("default=")+8))
`)
	assert.Greater(t, total, 0, "eval(document.location...) 应触发告警（漏报）")
}

// TestCodeInject_Positive_PugSSTI 验证 Pug 模板注入（字符串拼接用户输入到模板）触发 high 告警。
// 对应 irify/ts-sf-rules/cwe-94/code-inject/positive-1.js
func TestCodeInject_Positive_PugSSTI(t *testing.T) {
	rule := loadCodeInjectionRule(t)
	total, high := runOnFile(t, rule, "positive-1.js", `
const express = require('express')
var pug = require('pug');
const app = express()

app.post('/', (req, res) => {
    var input = req.query.username;
    var template = `+"`"+`
doctype
html
head
    title= 'Hello world'
body
    form(action='/' method='post')
        input#name.form-control(type='text)
        button.btn.btn-primary(type='submit') Submit
    p Hello `+"`"+` + input
    var fn = pug.compile(template);
    var html = fn();
    res.send(html);
})
`)
	assert.Greater(t, total, 0, "Pug 模板字符串拼接用户输入应触发告警（漏报）")
	assert.Greater(t, high, 0, "应触发 high 告警（直接将用户输入拼接入模板）")
}

// TestCodeInject_Positive_EvalReqQuery 验证 eval(req.query.code) 服务端直接注入触发 high 告警。
func TestCodeInject_Positive_EvalReqQuery(t *testing.T) {
	rule := loadCodeInjectionRule(t)
	total, high := runOnFile(t, rule, "eval_req_query.js", `
const express = require('express');
const app = express();

app.get('/run', (req, res) => {
    const result = eval(req.query.code);
    res.json({ result });
});
`)
	assert.Greater(t, total, 0, "eval(req.query.code) 应触发告警（漏报）")
	assert.Greater(t, high, 0, "eval(req.query.code) 应触发 high 告警")
}

// TestCodeInject_Positive_VmRunInNewContext 验证 vm.runInNewContext(req.query.code) 触发 high 告警。
func TestCodeInject_Positive_VmRunInNewContext(t *testing.T) {
	rule := loadCodeInjectionRule(t)
	total, high := runOnFile(t, rule, "vm_run.js", `
const vm = require('vm');
const express = require('express');
const app = express();

app.get('/eval', (req, res) => {
    const code = req.query.code;
    const result = vm.runInNewContext(code, {});
    res.json({ result });
});
`)
	assert.Greater(t, total, 0, "vm.runInNewContext(req.query.code) 应触发告警（漏报）")
	assert.Greater(t, high, 0, "应触发 high 告警")
}

// TestCodeInject_Positive_NewFunctionReqBody 验证 new Function(..., req.body.expression) 触发 high 告警。
func TestCodeInject_Positive_NewFunctionReqBody(t *testing.T) {
	rule := loadCodeInjectionRule(t)
	total, high := runOnFile(t, rule, "new_function.js", `
const express = require('express');
const app = express();
app.use(require('express').json());

app.post('/compute', (req, res) => {
    const fn = new Function('x', req.body.expression);
    res.json({ result: fn(42) });
});
`)
	assert.Greater(t, total, 0, "new Function(..., req.body.expression) 应触发告警（漏报）")
	assert.Greater(t, high, 0, "应触发 high 告警")
}

// ============================================================
// Negative: 用户输入作为模板变量而非模板本身（应零告警）
// ============================================================

// TestCodeInject_Negative_PugInputAsVariable 验证 Pug 模板中用户输入作为变量（非模板一部分）时不触发告警。
// 对应 irify/ts-sf-rules/cwe-94/code-inject/negative.js
func TestCodeInject_Negative_PugInputAsVariable(t *testing.T) {
	rule := loadCodeInjectionRule(t)
	total, _ := runOnFile(t, rule, "negative.js", `
const express = require('express')
var pug = require('pug');
const app = express()

app.post('/', (req, res) => {
    var input = req.query.username;
    var template = `+"`"+`
doctype
html
head
    title= 'Hello world'
body
    form(action='/' method='post')
        input#name.form-control(type='text)
        button.btn.btn-primary(type='submit') Submit
    p Hello #{username}`+"`"+`
    var fn = pug.compile(template);
    var html = fn({username: input});
    res.send(html);
})
`)
	assert.Equal(t, 0, total, "用户输入作为模板变量（非模板本身）不应触发告警（误报）")
}

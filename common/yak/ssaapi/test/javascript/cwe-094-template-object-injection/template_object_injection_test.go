package cwe094templateobjectinjection

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
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-094-template-object-injection/js-template-object-injection.sf")
	if !ok {
		t.Skip("js-template-object-injection.sf 不在当前构建的 embed FS 中，跳过测试")
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

// TestTemplateObjectInjection_BodyProfileDirectRender 验证 req.body.profile 直接传入 render 触发告警
func TestTemplateObjectInjection_BodyProfileDirectRender(t *testing.T) {
	rule := loadRule(t)
	code := `
var app = require('express')();
app.set('view engine', 'hbs');

app.post('/', function (req, res, next) {
    var profile = req.body.profile;
    res.render('index', profile);
});
`
	total := runOnFile(t, rule, "template_inject_hbs.js", code)
	assert.GreaterOrEqual(t, total, 1, "应检测到 req.body.profile 直接传入 render()")
}

// TestTemplateObjectInjection_BodyDirectRender 验证 req.body（整体）直接传入 render 触发告警。
func TestTemplateObjectInjection_BodyDirectRender(t *testing.T) {
	rule := loadRule(t)
	code := `
var app = require('express')();
app.set('view engine', 'ejs');

app.post('/profile', function (req, res) {
    var data = req.body;
    res.render('profile', data);
});
`
	total := runOnFile(t, rule, "template_inject_body.js", code)
	assert.GreaterOrEqual(t, total, 1, "应检测到 req.body 直接传入 render()")
}

// TestTemplateObjectInjection_QueryObjectRender 验证 req.query（整体）传入 render 触发告警。
func TestTemplateObjectInjection_QueryObjectRender(t *testing.T) {
	rule := loadRule(t)
	code := `
var app = require('express')();
app.set('view engine', 'hbs');

app.get('/greet', function (req, res) {
    res.render('hello', req.query);
});
`
	total := runOnFile(t, rule, "template_inject_query.js", code)
	assert.GreaterOrEqual(t, total, 1, "应检测到 req.query 直接传入 render()")
}

// TestTemplateObjectInjection_AliasedVariable 验证用户对象通过局部变量别名后传入 render 触发告警。
func TestTemplateObjectInjection_AliasedVariable(t *testing.T) {
	rule := loadRule(t)
	code := `
var app = require('express')();
app.set('view engine', 'hbs');

app.post('/dashboard', function (req, res) {
    var userData = req.body.user;
    var opts = userData;
    res.render('dashboard', opts);
});
`
	total := runOnFile(t, rule, "template_inject_alias.js", code)
	assert.GreaterOrEqual(t, total, 1, "应检测到通过变量别名传入的用户对象")
}

// TestTemplateObjectInjection_EtaEngine 验证 eta 引擎（脆弱列表中）触发告警。
func TestTemplateObjectInjection_EtaEngine(t *testing.T) {
	rule := loadRule(t)
	code := `
var app = require('express')();
app.set('view engine', 'eta');

app.post('/report', function (req, res) {
    var reportData = req.body.report;
    res.render('report', reportData);
});
`
	total := runOnFile(t, rule, "template_inject_eta.js", code)
	assert.GreaterOrEqual(t, total, 1, "应检测到 eta 引擎下的模板对象注入")
}

// TestTemplateObjectInjection_SafeExplicitObject 验证显式构造对象不触发告警（修复写法）。
func TestTemplateObjectInjection_SafeExplicitObject(t *testing.T) {
	rule := loadRule(t)
	code := `
var app = require('express')();
app.set('view engine', 'hbs');

app.post('/', function (req, res, next) {
    var profile = req.body.profile;
    res.render('index', {
        name: profile.name,
        location: profile.location
    });
});
`
	total := runOnFile(t, rule, "template_safe_explicit.js", code)
	assert.Equal(t, 0, total, "显式构造对象不应触发告警")
}

// TestTemplateObjectInjection_SafeHardcoded 验证硬编码静态对象不触发告警。
func TestTemplateObjectInjection_SafeHardcoded(t *testing.T) {
	rule := loadRule(t)
	code := `
var app = require('express')();
app.set('view engine', 'hbs');

app.get('/about', function (req, res) {
    res.render('about', { title: 'About Us', year: 2024 });
});
`
	total := runOnFile(t, rule, "template_safe_static.js", code)
	assert.Equal(t, 0, total, "硬编码静态对象不应触发告警")
}

// TestTemplateObjectInjection_SafeWhitelistFiltered 验证白名单过滤后的对象不触发告警。
func TestTemplateObjectInjection_SafeWhitelistFiltered(t *testing.T) {
	rule := loadRule(t)
	code := `
var app = require('express')();
app.set('view engine', 'ejs');

const ALLOWED_KEYS = ['name', 'bio', 'avatar'];

app.post('/profile', function (req, res) {
    const input = req.body.profile;
    const safeData = {};
    for (const key of ALLOWED_KEYS) {
        if (key in input) {
            safeData[key] = input[key];
        }
    }
    res.render('profile', safeData);
});
`
	total := runOnFile(t, rule, "template_safe_whitelist.js", code)
	assert.Equal(t, 0, total, "白名单过滤后的对象不应触发告警")
}

// TestTemplateObjectInjection_SafePugEngine 验证 pug 引擎（不在脆弱列表）不触发告警。
// 规则通过 engine-aware 链路绑定：$vulnSetCall(pug) 匹配失败 →
// $vulnApp 为空 → $confirmedRes 为空 → $taintedRender 为空。
func TestTemplateObjectInjection_SafePugEngine(t *testing.T) {
	rule := loadRule(t)
	code := `
var app = require('express')();
app.set('view engine', 'pug');

app.post('/profile', function (req, res) {
    res.render('profile', req.body);
});
`
	total := runOnFile(t, rule, "template_safe_pug.js", code)
	assert.Equal(t, 0, total, "pug 不在脆弱引擎列表中，不应触发告警")
}

// TestTemplateObjectInjection_SafeMustacheEngine 验证 mustache 引擎不触发告警。
func TestTemplateObjectInjection_SafeMustacheEngine(t *testing.T) {
	rule := loadRule(t)
	code := `
var app = require('express')();
app.set('view engine', 'mustache');

app.post('/page', function (req, res) {
    res.render('page', req.body);
});
`
	total := runOnFile(t, rule, "template_safe_mustache.js", code)
	assert.Equal(t, 0, total, "mustache 不在脆弱引擎列表中，不应触发告警")
}

// TestTemplateObjectInjection_SafeCustomRenderer 验证自定义 renderer 类不触发告警。
// 无 app.set() → gate 失败 → $confirmedRes 为空 → 不报告。
func TestTemplateObjectInjection_SafeCustomRenderer(t *testing.T) {
	rule := loadRule(t)
	code := `
class MyRenderer {
    render(template, data) {
        return template.replace('{{name}}', data.name);
    }
}
const renderer = new MyRenderer();
const result = renderer.render('Hello {{name}}', req.body);
`
	total := runOnFile(t, rule, "template_safe_custom.js", code)
	assert.Equal(t, 0, total, "没有配置 Express 脆弱模板引擎的自定义 renderer 不应触发告警")
}

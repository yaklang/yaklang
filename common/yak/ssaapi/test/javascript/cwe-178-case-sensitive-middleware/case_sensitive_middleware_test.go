package cwe178casesensitivemiddleware

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

// loadCaseSensitiveRule 从内置 embed FS 读取 js-case-sensitive-middleware-path.sf 规则内容。
func loadCaseSensitiveRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-178-improper-handling-of-case-sensitivity/js-case-sensitive-middleware-path.sf")
	if !ok {
		t.Skip("ecmascript/cwe-178-improper-handling-of-case-sensitivity/js-case-sensitive-middleware-path.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "js-case-sensitive-middleware-path.sf 内容为空")
	return content
}

// runOnFile 用单文件 VirtualFS 执行规则，返回总告警数。
func runOnFile(t *testing.T, ruleContent, filename, code string) int {
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

// ============================================================
// Positive: 不含 'i' 标志的正则中间件 + 字符串路径端点（应触发告警）
// ============================================================

// TestCaseSensitiveMiddleware_Positive_RegexWithoutIFlag 验证不含 'i' 标志的正则中间件 + 同路径字符串端点触发告警。
// 对应 irify/ts-sf-rules/cwe-178/case-sensitive-middleware-path/positive.js
func TestCaseSensitiveMiddleware_Positive_RegexWithoutIFlag(t *testing.T) {
	rule := loadCaseSensitiveRule(t)
	total := runOnFile(t, rule, "positive.js", `
const app = require('express')();

app.use(/\/admin\/.*/, (req, res, next) => {
    if (!req.user.isAdmin) {
        res.status(401).send('Unauthorized');
    } else {
        next();
    }
});

app.get('/admin/users/:id', (req, res) => {
    res.send(app.database.users[req.params.id]);
});
`)
	assert.Greater(t, total, 0, "不含 'i' 标志的正则 + 同前缀字符串路径端点应触发告警（漏报）")
}

// ============================================================
// Negative: 含 'i' 标志或路径前缀不匹配（不应触发告警）
// ============================================================

// TestCaseSensitiveMiddleware_Negative_RegexWithIFlag 验证含 'i' 标志的正则中间件不触发告警。
// 对应 irify/ts-sf-rules/cwe-178/case-sensitive-middleware-path/negative.js
func TestCaseSensitiveMiddleware_Negative_RegexWithIFlag(t *testing.T) {
	rule := loadCaseSensitiveRule(t)
	total := runOnFile(t, rule, "negative.js", `
const app = require('express')();

app.use(/\/admin\/.*/i, (req, res, next) => {
    if (!req.user.isAdmin) {
        res.status(401).send('Unauthorized');
    } else {
        next();
    }
});

app.get('/admin/users/:id', (req, res) => {
    res.send(app.database.users[req.params.id]);
});
`)
	assert.Equal(t, 0, total, "含 'i' 标志的正则不应触发告警（误报）")
}

// TestCaseSensitiveMiddleware_Negative_RegexPathNotOverlapping 验证正则路径与端点路径不重叠时不触发告警。
// 对应 irify/ts-sf-rules/cwe-178/case-sensitive-middleware-path/negative-1.js
// /\/go0p\/.*/ 不覆盖 /guest/users/:id，无绕过风险。
func TestCaseSensitiveMiddleware_Negative_RegexPathNotOverlapping(t *testing.T) {
	rule := loadCaseSensitiveRule(t)
	total := runOnFile(t, rule, "negative-1.js", `
const app = require('express')();

app.use(/\/go0p\/.*/, (req, res, next) => {
    if (!req.user.isAdmin) {
        res.status(401).send('Unauthorized');
    } else {
        next();
    }
});

app.get('/guest/users/:id', (req, res) => {
    res.send(app.database.users[req.params.id]);
});
`)
	assert.Equal(t, 0, total, "正则与端点路径不重叠时不应触发告警（误报）")
}

// TestCaseSensitiveMiddleware_Negative_NoStringEndpointUnderProtectedPath 验证受正则保护的路径下
// 没有字符串路径端点时不触发告警。
func TestCaseSensitiveMiddleware_Negative_NoStringEndpointUnderProtectedPath(t *testing.T) {
	rule := loadCaseSensitiveRule(t)
	total := runOnFile(t, rule, "no_string_endpoint.js", `
const app = require('express')();

app.use(/\/admin\/.*/, (req, res, next) => {
    if (!req.user.isAdmin) {
        res.status(401).send('Unauthorized');
    } else {
        next();
    }
});

// 没有 /admin/... 前缀的字符串路径端点
app.get('/public/data', (req, res) => {
    res.json({ data: 'public' });
});
`)
	assert.Equal(t, 0, total, "保护路径下无字符串端点时不应触发告警（误报）")
}

// TestCaseSensitiveMiddleware_Negative_NonExpressGetMethod 验证非 Express 对象的 .get() 方法不触发告警。
// 例如 Map.get()、axios.get() 等不应被错误识别为 Express 路由。
func TestCaseSensitiveMiddleware_Negative_NonExpressGetMethod(t *testing.T) {
	rule := loadCaseSensitiveRule(t)
	total := runOnFile(t, rule, "non_express_get.js", `
// Map.get() — 与 express 无关，不应触发告警
const cache = new Map();
cache.set('/admin/data', { secret: true });
const val = cache.get('/admin/data');
console.log(val);
`)
	assert.Equal(t, 0, total, "非 Express 的 .get() 方法不应触发告警（误报）")
}

package cwe598sensitivegetquery

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
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-598-sensitive-get-query/js-sensitive-get-query.sf")
	if !ok {
		t.Skip("js-sensitive-get-query.sf 不在当前构建的 embed FS 中，跳过测试")
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

// TestSensitiveGetQuery_PasswordInGet 验证通过 GET 请求传输密码触发告警。
func TestSensitiveGetQuery_PasswordInGet(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require('express');
const app = express();
app.use(require('body-parser').urlencoded({ extended: false }))

// BAD: sensitive information is read from query parameters
app.get('/login', (req, res) => {
    const user = req.query.user;
    const password = req.query.password;
    if (checkUser(user, password)) {
        res.send('Welcome');
    } else {
        res.send('Access denied');
    }
});
`
	total := runOnFile(t, rule, "login.js", code)
	assert.GreaterOrEqual(t, total, 1, "应检测到 GET 请求中的密码参数")
}

// TestSensitiveGetQuery_TokenAndApiKeyInGet 验证通过 GET 请求传输 token 和 api_key 触发告警。
func TestSensitiveGetQuery_TokenAndApiKeyInGet(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require('express');
const app = express();

// BAD: API token and key in GET query string
app.get('/api/data', (req, res) => {
    const token = req.query.token;
    const apiKey = req.query.api_key;
    if (validateToken(token)) {
        res.json({ data: 'secret data' });
    } else {
        res.status(401).send('Unauthorized');
    }
});
`
	total := runOnFile(t, rule, "token_api.js", code)
	assert.GreaterOrEqual(t, total, 1, "应检测到 GET 请求中的 token/api_key 参数")
}

// TestSensitiveGetQuery_SecretAndPrivateKey 验证 GET 中的 secret 和 private_key 触发告警。
func TestSensitiveGetQuery_SecretAndPrivateKey(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require('express');
const app = express();

// BAD: secret and private key in GET query string
app.get('/setup', (req, res) => {
    const secret = req.query.secret;
    const privateKey = req.query.private_key;
    configure(secret, privateKey);
    res.send('configured');
});
`
	total := runOnFile(t, rule, "secret.js", code)
	assert.GreaterOrEqual(t, total, 1, "应检测到 GET 请求中的 secret/private_key 参数")
}

// TestSensitiveGetQuery_KoaAuthToken 验证 Koa 框架下 ctx.query 中的敏感参数触发告警。
func TestSensitiveGetQuery_KoaAuthToken(t *testing.T) {
	rule := loadRule(t)
	code := `
const Koa = require('koa');
const app = new Koa();

app.use(async (ctx) => {
    // BAD: auth token in GET query params (Koa)
    const authToken = ctx.query.auth_token;
    const csrfToken = ctx.query.csrf_token;
    if (validateAuth(authToken)) {
        ctx.body = 'ok';
    }
});
`
	total := runOnFile(t, rule, "koa_auth.js", code)
	assert.GreaterOrEqual(t, total, 1, "应检测到 Koa ctx.query 中的敏感参数")
}

// TestSensitiveGetQuery_SafePostBody 验证 POST 请求体中的敏感数据不触发告警。
func TestSensitiveGetQuery_SafePostBody(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require('express');
const app = express();
app.use(require('body-parser').urlencoded({ extended: false }))

// GOOD: sensitive information is read from post body
app.post('/login', (req, res) => {
    const user = req.body.user;
    const password = req.body.password;
    const token = req.body.token;
    if (checkUser(user, password)) {
        res.send('Welcome');
    } else {
        res.send('Access denied');
    }
});
`
	total := runOnFile(t, rule, "safe_post.js", code)
	assert.Equal(t, 0, total, "POST 请求体中的敏感数据不应触发告警")
}

// TestSensitiveGetQuery_SafeNonSensitiveParams 验证 GET 中的非敏感参数不触发告警。
func TestSensitiveGetQuery_SafeNonSensitiveParams(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require('express');
const app = express();

// GOOD: non-sensitive search/filter params in GET
app.get('/search', (req, res) => {
    const keyword = req.query.q;
    const page = req.query.page;
    const user = req.query.user;
    const category = req.query.category;
    const sortBy = req.query.sort_by;
    res.json({ results: [] });
});
`
	total := runOnFile(t, rule, "safe_search.js", code)
	assert.Equal(t, 0, total, "非敏感 GET 参数不应触发告警")
}

// TestSensitiveGetQuery_SafeAuthorizationHeader 验证通过 Authorization 头传输 token 不触发告警。
func TestSensitiveGetQuery_SafeAuthorizationHeader(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require('express');
const app = express();

// GOOD: token transmitted via Authorization header
app.get('/api/data', (req, res) => {
    const authHeader = req.headers.authorization;
    const bearerToken = authHeader && authHeader.split(' ')[1];
    if (validateToken(bearerToken)) {
        res.json({ data: 'secret' });
    } else {
        res.status(401).send('Unauthorized');
    }
});
`
	total := runOnFile(t, rule, "safe_header.js", code)
	assert.Equal(t, 0, total, "Authorization 头中的 token 不应触发告警")
}

// TestSensitiveGetQuery_MultiSensitiveParams 验证多个敏感参数都被检测到。
func TestSensitiveGetQuery_MultiSensitiveParams(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require('express');
const app = express();

// BAD: multiple sensitive params in GET
app.get('/auth', (req, res) => {
    const password = req.query.password;
    const token = req.query.token;
    const secret = req.query.secret;
    doAuth(password, token, secret);
});
`
	total := runOnFile(t, rule, "multi_sensitive.js", code)
	assert.GreaterOrEqual(t, total, 3, "应检测到 3 个敏感 GET 参数")
}

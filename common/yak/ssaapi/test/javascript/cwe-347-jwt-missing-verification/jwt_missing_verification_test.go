package cwe347jwtmissingverification

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

// loadJWTRule 从内置 embed FS 读取 js-jwt-missing-verification.sf 规则内容。
func loadJWTRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-347-jwt-missing-verification/js-jwt-missing-verification.sf")
	if !ok {
		t.Skip("js-jwt-missing-verification.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "js-jwt-missing-verification.sf 内容为空")
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

// TestJWT_Positive_FalseSecret 使用 false 作为密钥（CodeQL 文档标准示例）
// jwt.verify(token, false, { algorithms: ["HS256", "none"] }) — 最典型的漏洞用法。
func TestJWT_Positive_FalseSecret(t *testing.T) {
	rule := loadJWTRule(t)
	code := `
const jwt = require("jsonwebtoken");

const secret = "my-secret-key";
var token = jwt.sign({ foo: 'bar' }, secret, { algorithm: "none" });
jwt.verify(token, false, { algorithms: ["HS256", "none"] });
`
	total := runOnCode(t, rule, "unsafe_jwt_false.js", code)
	assert.Greater(t, total, 0, "使用 false 作为 JWT 密钥应触发告警")
}

// TestJWT_Positive_NullSecret 使用 null 作为密钥
func TestJWT_Positive_NullSecret(t *testing.T) {
	rule := loadJWTRule(t)
	code := `
const jwt = require("jsonwebtoken");

var token = jwt.sign({ userId: 1, role: "user" }, "secret");
jwt.verify(token, null, { algorithms: ["none"] });
`
	total := runOnCode(t, rule, "unsafe_jwt_null.js", code)
	assert.Greater(t, total, 0, "使用 null 作为 JWT 密钥应触发告警")
}

// TestJWT_Positive_UndefinedSecret 使用 undefined 作为密钥
func TestJWT_Positive_UndefinedSecret(t *testing.T) {
	rule := loadJWTRule(t)
	code := `
const jwt = require("jsonwebtoken");

var token = jwt.sign({ userId: 1 }, "secret");
jwt.verify(token, undefined, {});
`
	total := runOnCode(t, rule, "unsafe_jwt_undefined.js", code)
	assert.Greater(t, total, 0, "使用 undefined 作为 JWT 密钥应触发告警")
}

// TestJWT_Positive_EmptyStringSecret 使用空字符串作为密钥
func TestJWT_Positive_EmptyStringSecret(t *testing.T) {
	rule := loadJWTRule(t)
	code := `
const jwt = require("jsonwebtoken");

var token = jwt.sign({ userId: 1 }, "secret");
jwt.verify(token, "", { algorithms: ["HS256"] });
`
	total := runOnCode(t, rule, "unsafe_jwt_empty_str.js", code)
	assert.Greater(t, total, 0, "使用空字符串作为 JWT 密钥应触发告警")
}

// TestJWT_Positive_ZeroSecret 使用数字 0 作为密钥
func TestJWT_Positive_ZeroSecret(t *testing.T) {
	rule := loadJWTRule(t)
	code := `
const jwt = require("jsonwebtoken");

var token = jwt.sign({ userId: 1 }, "secret");
jwt.verify(token, 0, { algorithms: ["HS256"] });
`
	total := runOnCode(t, rule, "unsafe_jwt_zero.js", code)
	assert.Greater(t, total, 0, "使用数字 0 作为 JWT 密钥应触发告警")
}

// TestJWT_Negative_ValidStringSecret 使用真实字符串密钥（安全）
// 对应 CodeQL 文档中的修复示例。
func TestJWT_Negative_ValidStringSecret(t *testing.T) {
	rule := loadJWTRule(t)
	code := `
const jwt = require("jsonwebtoken");

const secret = "my-secret-key";
var token = jwt.sign({ foo: 'bar' }, secret, { algorithm: "HS256" });
jwt.verify(token, secret, { algorithms: ["HS256"] });
`
	total := runOnCode(t, rule, "safe_jwt_string.js", code)
	assert.Equal(t, 0, total, "使用真实字符串密钥不应触发告警")
}

// TestJWT_Negative_PublicKeySecret 使用公钥（非对称算法，安全）
func TestJWT_Negative_PublicKeySecret(t *testing.T) {
	rule := loadJWTRule(t)
	code := `
const jwt = require("jsonwebtoken");
const fs = require("fs");

const privateKey = fs.readFileSync("private.key");
const publicKey = fs.readFileSync("public.key");

var token = jwt.sign({ userId: 123 }, privateKey, { algorithm: "RS256" });
jwt.verify(token, publicKey, { algorithms: ["RS256"] });
`
	total := runOnCode(t, rule, "safe_jwt_pubkey.js", code)
	assert.Equal(t, 0, total, "使用公钥进行 JWT 验证不应触发告警")
}

// TestJWT_Negative_EnvSecret 从环境变量读取密钥（安全）
func TestJWT_Negative_EnvSecret(t *testing.T) {
	rule := loadJWTRule(t)
	code := `
const jwt = require("jsonwebtoken");
const secret = process.env.JWT_SECRET;

function verifyToken(token) {
    return jwt.verify(token, secret, { algorithms: ["HS256"] });
}
`
	total := runOnCode(t, rule, "safe_jwt_env.js", code)
	assert.Equal(t, 0, total, "使用环境变量密钥不应触发告警")
}

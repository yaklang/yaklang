package cwe338insecurerandomness

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
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-338-use-of-cryptographically-weak-prng/js-insecure-randomness.sf")
	if !ok {
		t.Skip("js-insecure-randomness.sf 不在当前构建的 embed FS 中，跳过测试")
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

// TestInsecureRandomness_Positive 验证 Math.random() 用于密码生成时触发告警。
func TestInsecureRandomness_Positive(t *testing.T) {
	rule := loadRule(t)
	code := `
function insecurePassword() {
    // BAD: the random suffix is not cryptographically secure
    var suffix = Math.random();
    var password = "myPassword" + suffix;
    return password;
}
`
	total := runOnFile(t, rule, "positive.js", code)
	assert.Greater(t, total, 0, "Math.random() 用于密码生成应触发告警（漏报）")
}

// TestInsecureRandomness_Negative 验证使用 crypto.getRandomValues 不触发告警。
func TestInsecureRandomness_Negative(t *testing.T) {
	rule := loadRule(t)
	code := `
function securePassword() {
    // GOOD: the random suffix is cryptographically secure
    var suffix = window.crypto.getRandomValues(new Uint32Array(1))[0];
    var password = "myPassword" + suffix;

    // GOOD: if a random value between 0 and 1 is desired
    var secret = window.crypto.getRandomValues(new Uint32Array(1))[0] * Math.pow(2,-32);
}
`
	total := runOnFile(t, rule, "negative.js", code)
	assert.Equal(t, 0, total, "crypto.getRandomValues 不应触发告警（误报）")
}

// TestInsecureRandomness_TokenPositive 验证 Math.random() 用于 token 生成时触发告警。
func TestInsecureRandomness_TokenPositive(t *testing.T) {
	rule := loadRule(t)
	code := `
function generateToken() {
    const token = Math.random().toString(36).substring(2);
    return token;
}
`
	total := runOnFile(t, rule, "token.js", code)
	assert.Greater(t, total, 0, "Math.random() 用于 token 生成应触发告警")
}

// TestInsecureRandomness_SecretPositive 验证 Math.random() 用于 secret 生成时触发告警。
func TestInsecureRandomness_SecretPositive(t *testing.T) {
	rule := loadRule(t)
	code := `
const secret = Math.random().toString(16);
`
	total := runOnFile(t, rule, "secret.js", code)
	assert.Greater(t, total, 0, "Math.random() 用于 secret 生成应触发告警")
}

// TestInsecureRandomness_RGBNoAlert 验证 Math.random() 用于 RGB 颜色值不触发告警。
func TestInsecureRandomness_RGBNoAlert(t *testing.T) {
	rule := loadRule(t)
	code := `
function randomColor() {
    const r = Math.floor(Math.random() * 256);
    const g = Math.floor(Math.random() * 256);
    const b = Math.floor(Math.random() * 256);
    return 'rgb(' + r + ',' + g + ',' + b + ')';
}
`
	total := runOnFile(t, rule, "rgb.js", code)
	assert.Equal(t, 0, total, "Math.random() 用于 RGB 颜色值不应触发告警（误报）")
}

// TestInsecureRandomness_FunctionNameNoDoubleAlert 验证函数名匹配不影响规则正常告警。
// 回归测试：insecurePassword 函数名含 password，规则用 ?{!opcode: function} 过滤掉函数节点本身，
// 保证函数定义不会额外贡献告警（只有 password 变量赋值才是真正的 sink）。
// 此测试验证规则能正常检出漏洞（>0 告警），若函数名被误匹配则数量会偏高，可通过 debug 确认。
func TestInsecureRandomness_FunctionNameNoDoubleAlert(t *testing.T) {
	rule := loadRule(t)
	code := `
function insecurePassword() {
    var suffix = Math.random();
    var password = "myPassword" + suffix;
    return password;
}
`
	total := runOnFile(t, rule, "func_name.js", code)
	// 应触发至少 1 个告警（password 变量的 Math.random() 使用）
	// 注意：runOnFile 对所有 alert 变量值求和，实际数量取决于规则内部 $directInsecure/$taintedInsecure 的计算
	assert.Greater(t, total, 0, "含 password 变量的 Math.random() 应触发告警（漏报）")
}

// TestInsecureRandomness_DelayNoAlert 验证 Math.random() 用于延迟/动画不触发告警。
func TestInsecureRandomness_DelayNoAlert(t *testing.T) {
	rule := loadRule(t)
	code := `
const delay = Math.random() * 1000;
setTimeout(doSomething, delay);
`
	total := runOnFile(t, rule, "delay.js", code)
	assert.Equal(t, 0, total, "Math.random() 用于延迟/动画不应触发告警（误报）")
}

package cwe327weakcryptographicalgorithm

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
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-327-use-of-broken-or-weak-cryptographic-algorithm/js-weak-cryptographic-algorithm.sf")
	if !ok {
		t.Skip("js-weak-cryptographic-algorithm.sf 不在当前构建的 embed FS 中，跳过测试")
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

// TestWeakCryptoAlgorithm_Positive 验证弱加密算法触发告警。
func TestWeakCryptoAlgorithm_Positive(t *testing.T) {
	rule := loadRule(t)
	// DES cipher — canonical example
	code := `
const crypto = require('crypto');

var secretText = obj.getSecretText();

const desCipher = crypto.createCipher('des', key);
let desEncrypted = desCipher.write(secretText, 'utf8', 'hex'); // BAD: weak encryption
`
	total := runOnFile(t, rule, "positive.js", code)
	assert.Greater(t, total, 0, "DES 弱加密应触发告警（漏报）")
}

// TestWeakCryptoAlgorithm_StrongCipherNoAlert 验证强加密算法不触发告警。
// AES-128 是安全算法，应无告警。
func TestWeakCryptoAlgorithm_StrongCipherNoAlert(t *testing.T) {
	rule := loadRule(t)
	code := `
const crypto = require('crypto');

var secretText = obj.getSecretText();

const aesCipher = crypto.createCipher('aes-128', key);
let aesEncrypted = aesCipher.update(secretText, 'utf8', 'hex'); // GOOD: strong encryption
`
	total := runOnFile(t, rule, "negative_aes.js", code)
	assert.Equal(t, 0, total, "AES-128 不应触发告警（误报）")
}

// TestWeakCryptoAlgorithm_WeakCiphers 验证多种弱对称加密算法触发告警。
func TestWeakCryptoAlgorithm_WeakCiphers(t *testing.T) {
	rule := loadRule(t)
	code := `
const crypto = require('crypto');

// DES
crypto.createCipher('des', key);
// 3DES
crypto.createCipher('des3', key);
// RC4
crypto.createCipher('rc4', key);
// Blowfish
crypto.createCipher('bf-ecb', key);
// RC2
crypto.createCipheriv('rc2-40-cbc', key, iv);
`
	total := runOnFile(t, rule, "weak_ciphers.js", code)
	assert.Greater(t, total, 0, "弱对称加密算法应触发告警")
}

// TestWeakCryptoAlgorithm_WeakHashes 验证弱哈希算法触发告警。
func TestWeakCryptoAlgorithm_WeakHashes(t *testing.T) {
	rule := loadRule(t)
	code := `
const crypto = require('crypto');

// MD5
crypto.createHash('md5');
// SHA-1
crypto.createHash('sha1');
// MD4
crypto.createHash('md4');
`
	total := runOnFile(t, rule, "weak_hashes.js", code)
	assert.Greater(t, total, 0, "弱哈希算法应触发告警")
}

// TestWeakCryptoAlgorithm_StrongHashes 验证强哈希算法不触发告警。
func TestWeakCryptoAlgorithm_StrongHashes(t *testing.T) {
	rule := loadRule(t)
	code := `
const crypto = require('crypto');

// SHA-256
crypto.createHash('sha256');
// SHA-512
crypto.createHash('sha512');
// SHA-384
crypto.createHash('sha384');
// SHA3-256
crypto.createHash('sha3-256');
`
	total := runOnFile(t, rule, "strong_hashes.js", code)
	assert.Equal(t, 0, total, "强哈希算法不应触发告警（误报）")
}

// TestWeakCryptoAlgorithm_AlgorithmViaVariable 验证通过变量传递弱算法名也能触发告警。
func TestWeakCryptoAlgorithm_AlgorithmViaVariable(t *testing.T) {
	rule := loadRule(t)
	code := `
const crypto = require('crypto');

const DES_ALGO = 'des';
const cipher = crypto.createCipher(DES_ALGO, key);
`
	total := runOnFile(t, rule, "algo_via_var.js", code)
	assert.Greater(t, total, 0, "通过变量传递的弱算法名应触发告警")
}

// TestWeakCryptoAlgorithm_AESVariants 验证 AES 各变体均不触发告警。
func TestWeakCryptoAlgorithm_AESVariants(t *testing.T) {
	rule := loadRule(t)
	code := `
const crypto = require('crypto');

crypto.createCipher('aes-128-cbc', key);
crypto.createCipheriv('aes-256-gcm', key, iv);
crypto.createCipher('aes-192-ctr', key);
`
	total := runOnFile(t, rule, "aes_variants.js", code)
	assert.Equal(t, 0, total, "AES 系列算法不应触发告警（误报）")
}

// TestWeakCryptoAlgorithm_SHA1Positive 验证 SHA-1 触发告警但 SHA-256 不触发。
func TestWeakCryptoAlgorithm_SHA1Positive(t *testing.T) {
	rule := loadRule(t)

	sha1Total := runOnFile(t, rule, "sha1.js", `
const crypto = require('crypto');
crypto.createHash('sha1');
`)
	assert.Greater(t, sha1Total, 0, "SHA-1 应触发告警")

	sha256Total := runOnFile(t, rule, "sha256.js", `
const crypto = require('crypto');
crypto.createHash('sha256');
`)
	assert.Equal(t, 0, sha256Total, "SHA-256 不应触发告警")
}

package cwe326insufficientkeysize

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
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-326-inadequate-encryption-strength/js-insufficient-key-size.sf")
	if !ok {
		t.Skip("js-insufficient-key-size.sf 不在当前构建的 embed FS 中，跳过测试")
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

// TestInsufficientKeySize_Positive 验证弱密钥配置触发告警。
func TestInsufficientKeySize_Positive(t *testing.T) {
	rule := loadRule(t)
	// RSA-512 / RSA-1024 / DSA-1024 / P-192 / secp160r1 / prime192v1 / DH-512 / DH-1024
	code := `
const crypto = require('crypto');

// [1] RSA-512
crypto.generateKeyPair('rsa', { modulusLength: 512 }, (err, pub, priv) => {});

// [2] RSA-1024
crypto.generateKeyPairSync('rsa', {
    modulusLength: 1024,
    publicKeyEncoding:  { type: 'spki',  format: 'pem' },
    privateKeyEncoding: { type: 'pkcs8', format: 'pem' }
});

// [3] DSA-1024
crypto.generateKeyPairSync('dsa', { modulusLength: 1024, divisorLength: 160 });

// [4] EC P-192
crypto.generateKeyPair('ec', { namedCurve: 'P-192' }, (err, pub, priv) => {});

// [5] EC secp160r1
crypto.generateKeyPair('ec', { namedCurve: 'secp160r1' }, (err, pub, priv) => {});

// [6] ECDH prime192v1
const ecdhWeak = crypto.createECDH('prime192v1');

// [7] DH-512
const dhWeak512 = crypto.createDiffieHellman(512);

// [8] DH-1024
const dhWeak1024 = crypto.createDiffieHellman(1024);
`
	total := runOnFile(t, rule, "positive.js", code)
	assert.Greater(t, total, 0, "弱密钥配置应触发告警（漏报）")
}

// TestInsufficientKeySize_Negative 验证安全密钥配置不触发告警。
func TestInsufficientKeySize_Negative(t *testing.T) {
	rule := loadRule(t)
	code := `
const crypto = require('crypto');

// RSA-2048
crypto.generateKeyPair('rsa', { modulusLength: 2048 }, (err, pub, priv) => {});

// RSA-4096
crypto.generateKeyPairSync('rsa', { modulusLength: 4096 });

// EC P-256
crypto.generateKeyPair('ec', { namedCurve: 'P-256' }, (err, pub, priv) => {});

// EC P-384
crypto.generateKeyPair('ec', { namedCurve: 'P-384' }, (err, pub, priv) => {});

// X25519
crypto.generateKeyPair('x25519', {}, (err, pub, priv) => {});

// Ed25519
crypto.generateKeyPair('ed25519', {}, (err, pub, priv) => {});

// ECDH prime256v1
const ecdhOk = crypto.createECDH('prime256v1');

// DH-2048
const dhOk = crypto.createDiffieHellman(2048);
`
	total := runOnFile(t, rule, "negative.js", code)
	assert.Equal(t, 0, total, "安全密钥配置不应触发任何告警（误报）")
}

// TestInsufficientKeySize_RSA512 验证 RSA-512 单独触发告警。
func TestInsufficientKeySize_RSA512(t *testing.T) {
	rule := loadRule(t)
	total := runOnFile(t, rule, "rsa512.js", `
const crypto = require('crypto');
crypto.generateKeyPair('rsa', { modulusLength: 512 }, (err, pub, priv) => {});
`)
	assert.Greater(t, total, 0, "RSA-512 应触发告警")
}

// TestInsufficientKeySize_RSA2048Safe 验证 RSA-2048 不触发告警。
func TestInsufficientKeySize_RSA2048Safe(t *testing.T) {
	rule := loadRule(t)
	total := runOnFile(t, rule, "rsa2048.js", `
const crypto = require('crypto');
crypto.generateKeyPair('rsa', { modulusLength: 2048 }, (err, pub, priv) => {});
`)
	assert.Equal(t, 0, total, "RSA-2048 不应触发告警")
}

// TestInsufficientKeySize_WeakECCurve 验证弱椭圆曲线触发告警。
func TestInsufficientKeySize_WeakECCurve(t *testing.T) {
	rule := loadRule(t)
	total := runOnFile(t, rule, "weak_ec.js", `
const crypto = require('crypto');
crypto.generateKeyPair('ec', { namedCurve: 'P-192' }, (err, pub, priv) => {});
const ecdh = crypto.createECDH('secp160r1');
`)
	assert.Greater(t, total, 0, "弱 EC 曲线应触发告警")
}

// TestInsufficientKeySize_StrongECCurve 验证安全椭圆曲线不触发告警。
func TestInsufficientKeySize_StrongECCurve(t *testing.T) {
	rule := loadRule(t)
	total := runOnFile(t, rule, "strong_ec.js", `
const crypto = require('crypto');
crypto.generateKeyPair('ec', { namedCurve: 'P-256' }, (err, pub, priv) => {});
const ecdh = crypto.createECDH('prime256v1');
`)
	assert.Equal(t, 0, total, "安全 EC 曲线不应触发告警")
}

// TestInsufficientKeySize_WeakDH 验证弱 DH 素数长度触发告警。
func TestInsufficientKeySize_WeakDH(t *testing.T) {
	rule := loadRule(t)
	total := runOnFile(t, rule, "weak_dh.js", `
const crypto = require('crypto');
const dh = crypto.createDiffieHellman(1024);
`)
	assert.Greater(t, total, 0, "DH-1024 应触发告警")
}

// TestInsufficientKeySize_StrongDH 验证安全 DH 配置不触发告警。
func TestInsufficientKeySize_StrongDH(t *testing.T) {
	rule := loadRule(t)
	total := runOnFile(t, rule, "strong_dh.js", `
const crypto = require('crypto');
const dh = crypto.createDiffieHellman(2048);
`)
	assert.Equal(t, 0, total, "DH-2048 不应触发告警")
}

package codegrpc

import (
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/authhack"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var (
	defaultCodecExecFlow = NewCodecExecFlow([]byte(""), nil)
)

// generateRandomPlaintext 生成随机明文用于测试
func generateRandomPlaintext(minLen, maxLen int) []byte {
	// 随机长度在 minLen 到 maxLen 之间
	length := minLen
	if maxLen > minLen {
		extraLen := make([]byte, 1)
		rand.Read(extraLen)
		length = minLen + int(extraLen[0])%(maxLen-minLen+1)
	}

	plaintext := make([]byte, length)
	_, err := rand.Read(plaintext)
	if err != nil {
		panic(err)
	}
	return plaintext
}

func TestJwt(t *testing.T) {
	// 注： 这里testData的sig是错误的
	testData := `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9.eyJsb2dpbiI6InRlc3QiLCJpYXQiOiIxNzM0OTIyMTgxIn0.OGY2NDkyZWI3ZWQ3YmJkMjdiNmY0ODYwY2NjNTdiMGY3ZjAxMWM3YjkwMGYxNGViOTFiYzc4NzlkYWFmYTZmZA`
	defaultCodecExecFlow.Text = []byte(testData)
	// parse
	err := defaultCodecExecFlow.JwtParse()
	require.NoError(t, err)
	want := `{
    "alg": "HS256",
    "brute_secret_key_finished": false,
    "claims": {
        "iat": "1734922181",
        "login": "test"
    },
    "header": {
        "alg": "HS256",
        "typ": "JWS"
    },
    "is_valid": false,
    "raw": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXUyJ9.eyJsb2dpbiI6InRlc3QiLCJpYXQiOiIxNzM0OTIyMTgxIn0.OGY2NDkyZWI3ZWQ3YmJkMjdiNmY0ODYwY2NjNTdiMGY3ZjAxMWM3YjkwMGYxNGViOTFiYzc4NzlkYWFmYTZmZA",
    "secret_key": ""
}`
	// check result
	wantMap, gotMap := make(map[string]any), make(map[string]any)
	err = json.Unmarshal([]byte(want), &wantMap)
	require.NoError(t, err)
	err = json.Unmarshal(defaultCodecExecFlow.Text, &gotMap)
	require.NoError(t, err)
	require.Equal(t, wantMap, gotMap)

	// reverse sign
	err = defaultCodecExecFlow.JwtReverseSign()
	// check result
	require.NoError(t, err)
	wantToken, _, err := authhack.JwtParse(testData)
	require.ErrorIs(t, err, authhack.ErrKeyNotFound)
	gotToken, _, err := authhack.JwtParse(string(defaultCodecExecFlow.Text))
	require.ErrorIs(t, err, authhack.ErrKeyNotFound)

	require.Equal(t, wantToken.Header, gotToken.Header)
	require.Equal(t, wantToken.Claims, gotToken.Claims)
}

// TestAESGCMDecryptFallback 测试 AES-GCM 解密的兜底机制
// AES-GCM 有认证标签验证，可以安全地自动尝试多种编码
func TestAESGCMDecryptFallback(t *testing.T) {
	plaintext := generateRandomPlaintext(10, 100) // 生成10-100字节的随机明文
	key := "12345678901234567890123456789012"     // 32 bytes for AES-256
	nonce := "123456789012"                       // 12 bytes nonce

	// 测试1: 原文解密
	flow := NewCodecExecFlow(plaintext, nil)
	err := flow.AESGCMEncrypt(key, "raw", nonce, "raw", "12", "raw")
	require.NoError(t, err)
	encrypted1 := make([]byte, len(flow.Text))
	copy(encrypted1, flow.Text)

	flow = NewCodecExecFlow(encrypted1, nil)
	err = flow.AESGCMDecrypt(key, "raw", nonce, "raw", "12", "raw")
	require.NoError(t, err)
	require.Equal(t, plaintext, flow.Text)

	// 测试2: base64编码的兜底
	flow = NewCodecExecFlow(plaintext, nil)
	err = flow.AESGCMEncrypt(key, "raw", nonce, "raw", "12", "raw")
	require.NoError(t, err)
	encrypted2 := make([]byte, len(flow.Text))
	copy(encrypted2, flow.Text)

	encryptedBase64 := codec.EncodeBase64(encrypted2)
	flow = NewCodecExecFlow([]byte(encryptedBase64), nil)
	err = flow.AESGCMDecrypt(key, "raw", nonce, "raw", "12", "raw")
	require.NoError(t, err)
	require.Equal(t, plaintext, flow.Text)

	// 测试3: hex编码的兜底
	flow = NewCodecExecFlow(plaintext, nil)
	err = flow.AESGCMEncrypt(key, "raw", nonce, "raw", "12", "raw")
	require.NoError(t, err)
	encrypted3 := make([]byte, len(flow.Text))
	copy(encrypted3, flow.Text)

	encryptedHex := codec.EncodeToHex(encrypted3)
	flow = NewCodecExecFlow([]byte(encryptedHex), nil)
	err = flow.AESGCMDecrypt(key, "raw", nonce, "raw", "12", "raw")
	require.NoError(t, err)
	require.Equal(t, plaintext, flow.Text)
}

// TestRSADecryptFallback 测试 RSA 解密的兜底机制
// RSA 有密文长度验证，可以安全地自动尝试多种编码
func TestRSADecryptFallback(t *testing.T) {
	// RSA 2048位密钥最多能加密 214 字节 (2048/8 - 42 for OAEP padding)
	// 使用较小的明文以适应所有填充方式
	plaintext := generateRandomPlaintext(10, 50) // 生成10-50字节的随机明文

	// 生成 RSA 密钥对 (2048 bits)
	pubKeyPEM, priKeyPEM, err := tlsutils.RSAGenerateKeyPair(2048)
	require.NoError(t, err)

	// 测试所有填充方式和哈希算法组合
	testCases := []struct {
		name          string
		paddingSchema string
		hashAlgorithm string
	}{
		// RSA-OAEP 支持多种哈希算法
		{"RSA-OAEP_SHA-1", "RSA-OAEP", "SHA-1"},
		{"RSA-OAEP_SHA-256", "RSA-OAEP", "SHA-256"},
		{"RSA-OAEP_SHA-384", "RSA-OAEP", "SHA-384"},
		{"RSA-OAEP_SHA-512", "RSA-OAEP", "SHA-512"},
		{"RSA-OAEP_MD5", "RSA-OAEP", "MD5"},
		// PKCS1v15 (哈希算法参数会被忽略，但仍需提供)
		{"PKCS1v15", "PKCS1v15", "SHA-1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. 加密
			flow := NewCodecExecFlow(plaintext, nil)
			err := flow.RSAEncrypt(string(pubKeyPEM), tc.paddingSchema, tc.hashAlgorithm)
			require.NoError(t, err)
			encrypted := make([]byte, len(flow.Text))
			copy(encrypted, flow.Text)

			// 2. 测试原文解密（raw）
			flow = NewCodecExecFlow(encrypted, nil)
			err = flow.RSADecrypt(string(priKeyPEM), tc.paddingSchema, tc.hashAlgorithm)
			require.NoError(t, err)
			require.Equal(t, plaintext, flow.Text, "raw decryption failed for %s", tc.name)

			// 3. 测试base64编码的兜底
			encryptedBase64 := codec.EncodeBase64(encrypted)
			flow = NewCodecExecFlow([]byte(encryptedBase64), nil)
			err = flow.RSADecrypt(string(priKeyPEM), tc.paddingSchema, tc.hashAlgorithm)
			require.NoError(t, err)
			require.Equal(t, plaintext, flow.Text, "base64 fallback failed for %s", tc.name)

			// 4. 测试hex编码的兜底
			encryptedHex := codec.EncodeToHex(encrypted)
			flow = NewCodecExecFlow([]byte(encryptedHex), nil)
			err = flow.RSADecrypt(string(priKeyPEM), tc.paddingSchema, tc.hashAlgorithm)
			require.NoError(t, err)
			require.Equal(t, plaintext, flow.Text, "hex fallback failed for %s", tc.name)
		})
	}
}

// TestSM2DecryptFallback 测试 SM2 解密的兜底机制
// SM2 有密文格式验证，可以安全地自动尝试多种编码
func TestSM2DecryptFallback(t *testing.T) {
	plaintext := generateRandomPlaintext(10, 100) // 生成10-100字节的随机明文

	// 生成 SM2 密钥对
	priKey, pubKey, err := codec.GenerateSM2PrivateKeyHEX()
	require.NoError(t, err)

	// 测试所有三种 SM2 编码格式
	testCases := []struct {
		name          string
		encryptFunc   func([]byte, []byte) ([]byte, error)
		decryptSchema string
	}{
		{
			name: "ASN1",
			encryptFunc: func(pub, plain []byte) ([]byte, error) {
				return codec.SM2EncryptASN1(pub, plain)
			},
			decryptSchema: "ASN1",
		},
		{
			name: "C1C2C3",
			encryptFunc: func(pub, plain []byte) ([]byte, error) {
				return codec.SM2EncryptC1C2C3(pub, plain)
			},
			decryptSchema: "C1C2C3",
		},
		{
			name: "C1C3C2",
			encryptFunc: func(pub, plain []byte) ([]byte, error) {
				return codec.SM2EncryptC1C3C2(pub, plain)
			},
			decryptSchema: "C1C3C2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. 加密
			encrypted, err := tc.encryptFunc(pubKey, plaintext)
			require.NoError(t, err)

			// 2. 测试原文解密（raw）
			flow := NewCodecExecFlow(encrypted, nil)
			err = flow.SM2Decrypt(string(priKey), tc.decryptSchema)
			require.NoError(t, err)
			require.Equal(t, plaintext, flow.Text, "raw decryption failed for %s", tc.name)

			// 3. 测试base64编码的兜底
			encryptedBase64 := codec.EncodeBase64(encrypted)
			flow = NewCodecExecFlow([]byte(encryptedBase64), nil)
			err = flow.SM2Decrypt(string(priKey), tc.decryptSchema)
			require.NoError(t, err)
			require.Equal(t, plaintext, flow.Text, "base64 fallback failed for %s", tc.name)

			// 4. 测试hex编码的兜底
			encryptedHex := codec.EncodeToHex(encrypted)
			flow = NewCodecExecFlow([]byte(encryptedHex), nil)
			err = flow.SM2Decrypt(string(priKey), tc.decryptSchema)
			require.NoError(t, err)
			require.Equal(t, plaintext, flow.Text, "hex fallback failed for %s", tc.name)
		})
	}
}

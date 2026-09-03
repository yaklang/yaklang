package mysql

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// 认证插件名。
const (
	pluginNative          = "mysql_native_password"
	pluginCachingSHA2     = "caching_sha2_password"
	pluginSHA256Password  = "sha256_password"
	pluginClearPassword   = "mysql_clear_password"
	pluginOldPassword     = "mysql_old_password"
	pluginDefaultFallback = pluginNative
)

var errUnsupportedPlugin = errors.New("mysql: unsupported auth plugin")

// scrambleNative 计算 mysql_native_password 响应：
//
//	token = SHA1(password) XOR SHA1(nonce ++ SHA1(SHA1(password)))
func scrambleNative(nonce []byte, password string) []byte {
	if len(password) == 0 {
		return nil
	}
	stage1 := sha1.Sum([]byte(password))
	inner := sha1.Sum(stage1[:])
	h := sha1.New()
	h.Write(nonce)
	h.Write(inner[:])
	token := h.Sum(nil)
	for i := range token {
		token[i] ^= stage1[i]
	}
	return token
}

// scrambleCachingSHA2 计算 caching_sha2_password 快路径响应：
//
//	token = XOR(SHA256(password), SHA256(SHA256(SHA256(password)) ++ nonce))
func scrambleCachingSHA2(nonce []byte, password string) []byte {
	if len(password) == 0 {
		return nil
	}
	m1 := sha256.Sum256([]byte(password))
	m1h := sha256.Sum256(m1[:])
	h := sha256.New()
	h.Write(m1h[:])
	h.Write(nonce)
	m2 := h.Sum(nil)
	for i := range m1 {
		m1[i] ^= m2[i]
	}
	return m1[:]
}

// encryptPasswordRSA 用服务端 RSA 公钥加密 (password++NUL) XOR nonce，
// 用于 caching_sha2_password / sha256_password 的全量认证（明文信道场景）。
func encryptPasswordRSA(password string, nonce []byte, pub *rsa.PublicKey) ([]byte, error) {
	plain := make([]byte, len(password)+1)
	copy(plain, password)
	for i := range plain {
		plain[i] ^= nonce[i%len(nonce)]
	}
	return rsa.EncryptOAEP(sha1.New(), rand.Reader, pub, plain, nil)
}

// parseRSAPublicKey 解析服务端下发的 PEM 公钥。
func parseRSAPublicKey(pemData []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("mysql: no PEM data in server public key")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// 兼容个别实现使用 PKCS#1 格式。
		if pub1, err1 := x509.ParsePKCS1PublicKey(block.Bytes); err1 == nil {
			return pub1, nil
		}
		return nil, err
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("mysql: server public key is not RSA")
	}
	return pub, nil
}

// authResponse 按插件计算首轮认证响应。
// 返回 nil 响应表示空密码（服务端按空凭证处理）。
func authResponse(plugin string, nonce []byte, password string, allowCleartext bool) ([]byte, error) {
	switch plugin {
	case pluginNative:
		if len(nonce) < 20 {
			return nil, fmt.Errorf("%w: nonce too short for native password", errMalformedPacket)
		}
		return scrambleNative(nonce[:20], password), nil
	case pluginCachingSHA2, pluginSHA256Password:
		if len(nonce) < 20 {
			return nil, fmt.Errorf("%w: nonce too short for sha256 auth", errMalformedPacket)
		}
		if plugin == pluginSHA256Password && len(password) > 0 {
			// sha256_password 无缓存快路径：直接走公钥请求流程。
			return []byte{cachingReqKey}, nil
		}
		return scrambleCachingSHA2(nonce[:20], password), nil
	case pluginClearPassword:
		if !allowCleartext {
			return nil, fmt.Errorf("%w: %s over insecure channel", errUnsupportedPlugin, plugin)
		}
		return append([]byte(password), 0), nil
	case pluginOldPassword:
		// pre-4.1 的 double-3DES 算法已随 4.1 淘汰，不支持。
		return nil, fmt.Errorf("%w: %s (pre-4.1)", errUnsupportedPlugin, plugin)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlugin, plugin)
	}
}

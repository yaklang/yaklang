package tlsutils

import (
	"bytes"
	"crypto/rsa"
	"fmt"
	"strings"

	cryptorand "crypto/rand"
)

// RSAEncryptWithPKCS1v15Block 使用 RSA PKCS#1 v1.5 公钥加密数据（导出名为 codec.RSAEncryptWithPKCS1v15Block）
// 自动按公钥长度对超长明文分块加密，可处理任意长度的输入
//
// 参数:
//   - pubKeyPem: PEM 格式的 RSA 公钥
//   - data: 待加密的明文字节
//
// 返回值:
//   - 密文字节
//   - 错误信息（公钥解析失败或加密失败时返回）
//
// Example:
// ```
// pub, pri = tls.GenerateRSA2048KeyPair()~
// ciphertext = codec.RSAEncryptWithPKCS1v15Block(pub, "hello yak")~
// plaintext = codec.RSADecryptWithPKCS1v15Block(pri, ciphertext)~
// println(string(plaintext))
// assert string(plaintext) == "hello yak", "PKCS1v15 block roundtrip should recover plaintext"
// ```
func RSAEncryptWithPKCS1v15Block(pubKeyPem string, data []byte) ([]byte, error) {
	pub, err := GetRSAPubKey([]byte(pubKeyPem))
	if err != nil {
		return nil, fmt.Errorf("parse public key failed: %w", err)
	}

	if len(data) == 0 {
		return []byte{}, nil
	}

	maxPlainBlockSize := pub.Size() - 11
	if maxPlainBlockSize <= 0 {
		return nil, fmt.Errorf("invalid rsa public key size: %d", pub.Size())
	}

	var encrypted bytes.Buffer
	for start := 0; start < len(data); start += maxPlainBlockSize {
		end := start + maxPlainBlockSize
		if end > len(data) {
			end = len(data)
		}

		chunk, err := rsa.EncryptPKCS1v15(cryptorand.Reader, pub, data[start:end])
		if err != nil {
			return nil, fmt.Errorf("encrypt block %d failed: %w", start/maxPlainBlockSize, err)
		}
		encrypted.Write(chunk)
	}
	return encrypted.Bytes(), nil
}

// RSADecryptWithPKCS1v15Block 使用 RSA PKCS#1 v1.5 私钥解密数据（导出名为 codec.RSADecryptWithPKCS1v15Block）
// 自动按私钥长度对密文分块解密，与 codec.RSAEncryptWithPKCS1v15Block 配对使用
//
// 参数:
//   - privKeyPem: PEM 格式的 RSA 私钥
//   - ciphertext: 待解密的密文字节
//
// 返回值:
//   - 解密得到的明文字节
//   - 错误信息（私钥解析失败、密文长度非法或解密失败时返回）
//
// Example:
// ```
// pub, pri = tls.GenerateRSA2048KeyPair()~
// ciphertext = codec.RSAEncryptWithPKCS1v15Block(pub, "hello yak")~
// plaintext = codec.RSADecryptWithPKCS1v15Block(pri, ciphertext)~
// println(string(plaintext))
// assert string(plaintext) == "hello yak", "PKCS1v15 block roundtrip should recover plaintext"
// ```
func RSADecryptWithPKCS1v15Block(privKeyPem string, ciphertext []byte) ([]byte, error) {
	pri, err := GetRSAPrivateKey([]byte(privKeyPem))
	if err != nil {
		return nil, fmt.Errorf("parse private key failed: %w", err)
	}

	if len(ciphertext) == 0 {
		return []byte{}, nil
	}

	blockSize := pri.Size()
	if blockSize <= 0 {
		return nil, fmt.Errorf("invalid rsa private key size: %d", pri.Size())
	}
	if len(ciphertext)%blockSize != 0 {
		return nil, fmt.Errorf("invalid ciphertext length %d: not multiple of rsa block size %d", len(ciphertext), blockSize)
	}

	var plaintext bytes.Buffer
	for start := 0; start < len(ciphertext); start += blockSize {
		chunk, err := rsa.DecryptPKCS1v15(cryptorand.Reader, pri, ciphertext[start:start+blockSize])
		if err != nil {
			return nil, fmt.Errorf("decrypt block %d failed: %w", start/blockSize, err)
		}
		plaintext.Write(chunk)
	}
	return plaintext.Bytes(), nil
}

// RSASignWithPKCS1v15Digest 使用 RSA PKCS#1 v1.5 私钥对数据做摘要签名（导出名为 codec.RSASignWithPKCS1v15Digest）
// 支持 sha256 与 sha512 两种摘要算法
//
// 参数:
//   - privKeyPem: PEM 格式的 RSA 私钥
//   - data: 待签名的原始数据
//   - algo: 摘要算法名，支持 "sha256"、"sha512"（大小写与写法不敏感）
//
// 返回值:
//   - 签名字节
//   - 错误信息（算法不支持或签名失败时返回）
//
// Example:
// ```
// pub, pri = tls.GenerateRSA2048KeyPair()~
// signature = codec.RSASignWithPKCS1v15Digest(pri, "hello yak", "sha256")~
// valid = codec.RSAVerifyWithPKCS1v15Digest(pub, "hello yak", signature, "sha256")~
// println(valid)
// assert valid == true, "signature should be verified as valid"
// ```
func RSASignWithPKCS1v15Digest(privKeyPem string, data []byte, algo string) ([]byte, error) {
	switch normalizeRSASignAlgo(algo) {
	case "sha256":
		return PemSignSha256WithRSA([]byte(privKeyPem), data)
	case "sha512":
		return PemSignSha512WithRSA([]byte(privKeyPem), data)
	default:
		return nil, fmt.Errorf("unsupported rsa sign algorithm: %s", algo)
	}
}

// RSAVerifyWithPKCS1v15Digest 使用 RSA PKCS#1 v1.5 公钥验证摘要签名（导出名为 codec.RSAVerifyWithPKCS1v15Digest）
// 与 codec.RSASignWithPKCS1v15Digest 配对使用，支持 sha256 与 sha512
//
// 参数:
//   - pubKeyPem: PEM 格式的 RSA 公钥
//   - data: 被签名的原始数据
//   - signature: 待验证的签名字节
//   - algo: 摘要算法名，支持 "sha256"、"sha512"
//
// 返回值:
//   - 验证是否通过（true 表示签名有效）
//   - 错误信息（算法不支持或验证出错时返回）
//
// Example:
// ```
// pub, pri = tls.GenerateRSA2048KeyPair()~
// signature = codec.RSASignWithPKCS1v15Digest(pri, "hello yak", "sha256")~
// valid = codec.RSAVerifyWithPKCS1v15Digest(pub, "hello yak", signature, "sha256")~
// println(valid)
// assert valid == true, "signature should be verified as valid"
// ```
func RSAVerifyWithPKCS1v15Digest(pubKeyPem string, data []byte, signature []byte, algo string) (bool, error) {
	var err error
	switch normalizeRSASignAlgo(algo) {
	case "sha256":
		err = PemVerifySignSha256WithRSA([]byte(pubKeyPem), data, signature)
	case "sha512":
		err = PemVerifySignSha512WithRSA([]byte(pubKeyPem), data, signature)
	default:
		return false, fmt.Errorf("unsupported rsa verify algorithm: %s", algo)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// RSAEncryptWithJSEncryptStyle 以兼容前端 JSEncrypt 库的方式做 RSA 加密（导出名为 codec.RSAEncryptWithJSEncryptStyle）
// 等价于 PKCS#1 v1.5 分块加密，便于与使用 JSEncrypt 的前端互通
//
// 参数:
//   - pubKeyPem: PEM 格式的 RSA 公钥
//   - data: 待加密的明文字节
//
// 返回值:
//   - 密文字节
//   - 错误信息（公钥解析失败或加密失败时返回）
//
// Example:
// ```
// pub, pri = tls.GenerateRSA2048KeyPair()~
// ciphertext = codec.RSAEncryptWithJSEncryptStyle(pub, "hello yak")~
// plaintext = codec.RSADecryptWithJSEncryptStyle(pri, ciphertext)~
// println(string(plaintext))
// assert string(plaintext) == "hello yak", "JSEncrypt-style roundtrip should recover plaintext"
// ```
func RSAEncryptWithJSEncryptStyle(pubKeyPem string, data []byte) ([]byte, error) {
	return RSAEncryptWithPKCS1v15Block(pubKeyPem, data)
}

// RSADecryptWithJSEncryptStyle 以兼容前端 JSEncrypt 库的方式做 RSA 解密（导出名为 codec.RSADecryptWithJSEncryptStyle）
// 等价于 PKCS#1 v1.5 分块解密，与 codec.RSAEncryptWithJSEncryptStyle 配对使用
//
// 参数:
//   - privKeyPem: PEM 格式的 RSA 私钥
//   - ciphertext: 待解密的密文字节
//
// 返回值:
//   - 解密得到的明文字节
//   - 错误信息（私钥解析失败或解密失败时返回）
//
// Example:
// ```
// pub, pri = tls.GenerateRSA2048KeyPair()~
// ciphertext = codec.RSAEncryptWithJSEncryptStyle(pub, "hello yak")~
// plaintext = codec.RSADecryptWithJSEncryptStyle(pri, ciphertext)~
// println(string(plaintext))
// assert string(plaintext) == "hello yak", "JSEncrypt-style roundtrip should recover plaintext"
// ```
func RSADecryptWithJSEncryptStyle(privKeyPem string, ciphertext []byte) ([]byte, error) {
	return RSADecryptWithPKCS1v15Block(privKeyPem, ciphertext)
}

func normalizeRSASignAlgo(algo string) string {
	normalized := strings.ToLower(strings.TrimSpace(algo))
	if normalized == "" {
		return "sha256"
	}

	var compact strings.Builder
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			compact.WriteRune(r)
		}
	}

	switch compact.String() {
	case "sha256", "sha256withrsa", "rsasha256":
		return "sha256"
	case "sha512", "sha512withrsa", "rsasha512":
		return "sha512"
	default:
		return normalized
	}
}

package codec

import (
	"bytes"
	"math/big"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/gmsm/sm2"
	"github.com/yaklang/yaklang/common/gmsm/x509"

	cryptoRand "crypto/rand"
)

// isHexString 检查字节数组是否为hex字符串格式
func isHexString(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// 检查是否全部为hex字符
	for _, b := range data {
		if !((b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')) {
			return false
		}
	}
	return true
}

// parsePrivateKey 解析私钥，支持PEM格式、hex字符串和已解码的字节数组
func parsePrivateKey(priKey []byte, password []byte) (*sm2.PrivateKey, error) {
	if len(priKey) == 0 {
		return nil, errors.New("private key is empty")
	}

	if bytes.HasPrefix(priKey, []byte("---")) {
		// PEM格式
		return x509.ReadPrivateKeyFromPem(priKey, password)
	} else if isHexString(priKey) {
		// hex字符串格式
		return x509.ReadPrivateKeyFromHex(string(priKey))
	} else {
		// 已解码的字节数组，直接构造私钥
		// 对于SM2私钥，如果是32字节的原始字节数组，直接使用
		if len(priKey) == 32 {
			return readPrivateKeyFromBytes(priKey)
		}
		// 如果不是32字节，可能是其他格式，尝试hex解析
		return x509.ReadPrivateKeyFromHex(string(priKey))
	}
}

// parsePublicKey 解析公钥，支持PEM格式、hex字符串和已解码的字节数组
func parsePublicKey(pubKey []byte) (*sm2.PublicKey, error) {
	pubKey = bytes.TrimSpace(pubKey)
	if len(pubKey) == 0 {
		return nil, errors.New("public key is empty")
	}

	if bytes.HasPrefix(pubKey, []byte("---")) {
		// PEM格式
		return x509.ReadPublicKeyFromPem(pubKey)
	} else if isHexString(pubKey) {
		// hex字符串格式
		return x509.ReadPublicKeyFromHex(string(pubKey))
	} else {
		// 已解码的字节数组，直接构造公钥
		// 对于SM2公钥，如果是64或65字节的原始字节数组，直接使用
		if len(pubKey) == 64 || len(pubKey) == 65 {
			return readPublicKeyFromBytes(pubKey)
		}
		// 如果不是预期长度，可能是其他格式，尝试hex解析
		return x509.ReadPublicKeyFromHex(string(pubKey))
	}
}

// readPrivateKeyFromBytes 从字节数组构造SM2私钥
func readPrivateKeyFromBytes(d []byte) (*sm2.PrivateKey, error) {
	c := sm2.P256Sm2()
	k := new(big.Int).SetBytes(d)
	params := c.Params()
	one := new(big.Int).SetInt64(1)
	n := new(big.Int).Sub(params.N, one)
	if k.Cmp(n) >= 0 {
		return nil, errors.New("privateKey's D is overflow.")
	}
	priv := new(sm2.PrivateKey)
	priv.PublicKey.Curve = c
	priv.D = k
	priv.PublicKey.X, priv.PublicKey.Y = c.ScalarBaseMult(k.Bytes())
	return priv, nil
}

// readPublicKeyFromBytes 从字节数组构造SM2公钥
func readPublicKeyFromBytes(q []byte) (*sm2.PublicKey, error) {
	if len(q) == 65 && q[0] == byte(0x04) {
		q = q[1:]
	}
	if len(q) != 64 {
		return nil, errors.New("publicKey is not uncompressed.")
	}
	pub := new(sm2.PublicKey)
	pub.Curve = sm2.P256Sm2()
	pub.X = new(big.Int).SetBytes(q[:32])
	pub.Y = new(big.Int).SetBytes(q[32:])
	return pub, nil
}

// GenerateSM2PrivateKeyPEM 生成一对国密 SM2 密钥(PEM 文本格式)
// 返回值:
//   - []byte: SM2 私钥(PEM 文本)
//   - []byte: SM2 公钥(PEM 文本)
//   - error: 生成失败时返回的错误
//
// Example:
// ```
// // VARS: 生成 PEM 格式 SM2 密钥对(返回顺序: 私钥, 公钥)
// priv, pub = codec.Sm2GeneratePemKeyPair()~
// // STDOUT: 打印私钥是否为 PEM 文本
// println(str.HasPrefix(string(priv), "-----BEGIN"))   // OUT: true
// // assert: 锁定结论(生成非空密钥对)
// assert len(priv) > 0 && len(pub) > 0, "Sm2GeneratePemKeyPair should produce keypair"
// ```
func GenerateSM2PrivateKeyPEM() ([]byte, []byte, error) {
	pkey, err := sm2.GenerateKey(cryptoRand.Reader)
	if err != nil {
		return nil, nil, errors.Wrap(err, "sm2.GenerateKey(cryptoRand.Reader)")
	}
	pKeyBytes, err := x509.WritePrivateKeyToPem(pkey, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "write sm2.privateKey to pem")
	}

	pubKeyBytes, err := x509.WritePublicKeyToPem(pkey.Public().(*sm2.PublicKey))
	if err != nil {
		return nil, nil, errors.Wrap(err, "write sm2.publicKey to pem")
	}
	return pKeyBytes, pubKeyBytes, nil
}

// GenerateSM2PrivateKeyHEX 生成一对国密 SM2 密钥(HEX 文本格式)
// 返回值:
//   - []byte: SM2 私钥(HEX 文本)
//   - []byte: SM2 公钥(HEX 文本)
//   - error: 生成失败时返回的错误
//
// Example:
// ```
// // VARS: 生成 HEX 格式 SM2 密钥对(返回顺序: 私钥, 公钥)
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// // STDOUT: 打印是否生成非空密钥对
// println(len(priv) > 0 && len(pub) > 0)   // OUT: true
// // assert: 锁定结论(生成非空密钥对)
// assert len(priv) > 0 && len(pub) > 0, "Sm2GenerateHexKeyPair should produce keypair"
// ```
func GenerateSM2PrivateKeyHEX() ([]byte, []byte, error) {
	for i := 0; i < 10; i++ {
		pkey, err := sm2.GenerateKey(cryptoRand.Reader)
		if err != nil {
			return nil, nil, errors.Wrap(err, "sm2.GenerateKey(cryptoRand.Reader)")
		}
		pKeyBytes := []byte(x509.WritePrivateKeyToHex(pkey))
		pubKeyBytes := []byte(x509.WritePublicKeyToHex(pkey.Public().(*sm2.PublicKey)))

		data, err := SM2EncryptC1C2C3(pubKeyBytes, []byte("abc"))
		if err != nil {
			continue
		}
		decData, err := SM2DecryptC1C2C3(pKeyBytes, data)
		if err != nil {
			continue
		}
		if string(decData) != "abc" {
			continue
		}
		return pKeyBytes, pubKeyBytes, nil
	}

	return nil, nil, errors.New("generate sm2 private key failed")
}

// preprocessCiphertext 预处理密文，自动添加缺失的0x04前缀
func preprocessCiphertext(data []byte) []byte {
	// 如果密文长度符合无前缀格式（96 + 密文数据长度），且第一个字节不是0x04
	if len(data) >= 96 && data[0] != 0x04 {
		// 自动添加0x04前缀
		result := make([]byte, len(data)+1)
		result[0] = 0x04
		copy(result[1:], data)
		return result
	}
	return data
}

// SM2EncryptC1C2C3 使用国密 SM2 公钥按 C1C2C3 密文排列加密数据
// 注意：Sm2Encrypt 和 Sm2EncryptC1C2C3 是同一个函数的别名
// 参数:
//   - pubKey: SM2 公钥(支持 PEM/HEX/原始字节)
//   - data: 待加密的数据字节
//
// 返回值:
//   - []byte: 加密后的密文字节(每次随机，结果不固定)
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: 生成密钥对并做 C1C2C3 加解密往返
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// ct = codec.Sm2EncryptC1C2C3(pub, "secret")~
// pt = codec.Sm2DecryptC1C2C3(priv, ct)~
// // STDOUT: 打印解密还原后的明文
// println(string(pt))   // OUT: secret
// // assert: 锁定结论(SM2 C1C2C3 加解密往返一致)
// assert string(pt) == "secret", "SM2 C1C2C3 should round-trip"
// ```
func SM2EncryptC1C2C3(pubKey []byte, data []byte) ([]byte, error) {
	pub, err := parsePublicKey(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.publicKey")
	}

	results, err := sm2.Encrypt(pub, data, cryptoRand.Reader, sm2.C1C2C3)
	if err != nil {
		return nil, errors.Wrap(err, "sm2.Encrypt[C1C2C3] with pubkey")
	}
	return results, nil
}

// SM2DecryptC1C2C3 使用国密 SM2 私钥按 C1C2C3 密文排列解密数据
// 注意：Sm2Decrypt 和 Sm2DecryptC1C2C3 是同一个函数的别名
// 参数:
//   - priKey: SM2 私钥(支持 PEM/HEX/原始字节)
//   - data: 待解密的密文字节
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 生成密钥对并做 C1C2C3 加解密往返
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// ct = codec.Sm2EncryptC1C2C3(pub, "secret")~
// pt = codec.Sm2DecryptC1C2C3(priv, ct)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: secret
// // assert: 锁定结论(SM2 C1C2C3 解密还原一致)
// assert string(pt) == "secret", "SM2 C1C2C3 decrypt should recover plaintext"
// ```
func SM2DecryptC1C2C3(priKey []byte, data []byte) ([]byte, error) {
	return SM2DecryptC1C2C3WithPassword(priKey, data, nil)
}

// SM2DecryptC1C2C3WithPassword 使用带密码保护的国密 SM2 私钥按 C1C2C3 密文排列解密数据
// 参数:
//   - priKey: SM2 私钥(支持 PEM/HEX/原始字节，可为加密私钥)
//   - data: 待解密的密文字节
//   - password: 私钥保护密码，未加密时传 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 未加密私钥时 password 传 nil
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// ct = codec.Sm2EncryptC1C2C3(pub, "secret")~
// pt = codec.Sm2DecryptC1C2C3WithPassword(priv, ct, nil)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: secret
// // assert: 锁定结论(带密码接口在 nil 密码下也能解密)
// assert string(pt) == "secret", "SM2 C1C2C3 with password(nil) should recover plaintext"
// ```
func SM2DecryptC1C2C3WithPassword(priKey []byte, data []byte, password []byte) ([]byte, error) {
	pri, err := parsePrivateKey(priKey, password)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.privateKey")
	}

	// 预处理密文，自动添加缺失的0x04前缀
	processedData := preprocessCiphertext(data)

	results, err := sm2.Decrypt(pri, processedData, sm2.C1C2C3)
	if err != nil {
		return nil, errors.Wrap(err, "sm2.Decrypt[C1C2C3] with prikey")
	}
	return results, nil
}

// SM2EncryptC1C3C2 使用国密 SM2 公钥按 C1C3C2 密文排列加密数据
// 参数:
//   - pubKey: SM2 公钥(支持 PEM/HEX/原始字节)
//   - data: 待加密的数据字节
//
// 返回值:
//   - []byte: 加密后的密文字节(每次随机，结果不固定)
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: 生成密钥对并做 C1C3C2 加解密往返
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// ct = codec.Sm2EncryptC1C3C2(pub, "secret")~
// pt = codec.Sm2DecryptC1C3C2(priv, ct)~
// // STDOUT: 打印解密还原后的明文
// println(string(pt))   // OUT: secret
// // assert: 锁定结论(SM2 C1C3C2 加解密往返一致)
// assert string(pt) == "secret", "SM2 C1C3C2 should round-trip"
// ```
func SM2EncryptC1C3C2(pubKey []byte, data []byte) ([]byte, error) {
	pub, err := parsePublicKey(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.publicKey")
	}

	results, err := sm2.Encrypt(pub, data, cryptoRand.Reader, sm2.C1C3C2)
	if err != nil {
		return nil, errors.Wrap(err, "sm2.Encrypt[C1C3C2] with pubkey")
	}
	return results, nil
}

// SM2DecryptC1C3C2 使用国密 SM2 私钥按 C1C3C2 密文排列解密数据
// 参数:
//   - priKey: SM2 私钥(支持 PEM/HEX/原始字节)
//   - data: 待解密的密文字节
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 生成密钥对并做 C1C3C2 加解密往返
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// ct = codec.Sm2EncryptC1C3C2(pub, "secret")~
// pt = codec.Sm2DecryptC1C3C2(priv, ct)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: secret
// // assert: 锁定结论(SM2 C1C3C2 解密还原一致)
// assert string(pt) == "secret", "SM2 C1C3C2 decrypt should recover plaintext"
// ```
func SM2DecryptC1C3C2(priKey []byte, data []byte) ([]byte, error) {
	return SM2DecryptC1C3C2WithPassword(priKey, data, nil)
}

// SM2DecryptC1C3C2WithPassword 使用带密码保护的国密 SM2 私钥按 C1C3C2 密文排列解密数据
// 参数:
//   - priKey: SM2 私钥(支持 PEM/HEX/原始字节，可为加密私钥)
//   - data: 待解密的密文字节
//   - password: 私钥保护密码，未加密时传 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 未加密私钥时 password 传 nil
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// ct = codec.Sm2EncryptC1C3C2(pub, "secret")~
// pt = codec.Sm2DecryptC1C3C2WithPassword(priv, ct, nil)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: secret
// // assert: 锁定结论(带密码接口在 nil 密码下也能解密)
// assert string(pt) == "secret", "SM2 C1C3C2 with password(nil) should recover plaintext"
// ```
func SM2DecryptC1C3C2WithPassword(priKey []byte, data []byte, password []byte) ([]byte, error) {
	pri, err := parsePrivateKey(priKey, password)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.privateKey")
	}

	// 预处理密文，自动添加缺失的0x04前缀
	processedData := preprocessCiphertext(data)

	results, err := sm2.Decrypt(pri, processedData, sm2.C1C3C2)
	if err != nil {
		return nil, errors.Wrap(err, "sm2.Decrypt[C1C3C2] with pubkey")
	}
	return results, nil
}

// SM2EncryptASN1 使用国密 SM2 公钥按 ASN.1 编码格式加密数据
// 注意：Sm2EncryptAsn1 是本函数的导出名
// 参数:
//   - pubKey: SM2 公钥(支持 PEM/HEX/原始字节)
//   - data: 待加密的数据字节
//
// 返回值:
//   - []byte: ASN.1 编码的密文字节(每次随机，结果不固定)
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: 生成密钥对并做 ASN.1 加解密往返
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// ct = codec.Sm2EncryptAsn1(pub, "secret")~
// pt = codec.Sm2DecryptAsn1(priv, ct)~
// // STDOUT: 打印解密还原后的明文
// println(string(pt))   // OUT: secret
// // assert: 锁定结论(SM2 ASN.1 加解密往返一致)
// assert string(pt) == "secret", "SM2 ASN1 should round-trip"
// ```
func SM2EncryptASN1(pubKey []byte, data []byte) ([]byte, error) {
	pub, err := parsePublicKey(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.publicKey")
	}

	return sm2.EncryptAsn1(pub, data, cryptoRand.Reader)
}

// SM2DecryptASN1 使用国密 SM2 私钥按 ASN.1 编码格式解密数据
// 注意：Sm2DecryptAsn1 是本函数的导出名
// 参数:
//   - priKey: SM2 私钥(支持 PEM/HEX/原始字节)
//   - data: 待解密的 ASN.1 编码密文字节
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 生成密钥对并做 ASN.1 加解密往返
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// ct = codec.Sm2EncryptAsn1(pub, "secret")~
// pt = codec.Sm2DecryptAsn1(priv, ct)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: secret
// // assert: 锁定结论(SM2 ASN.1 解密还原一致)
// assert string(pt) == "secret", "SM2 ASN1 decrypt should recover plaintext"
// ```
func SM2DecryptASN1(priKey []byte, data []byte) ([]byte, error) {
	return SM2DecryptASN1WithPassword(priKey, data, nil)
}

// SM2DecryptASN1WithPassword 使用带密码保护的国密 SM2 私钥按 ASN.1 编码格式解密数据
// 注意：Sm2DecryptAsn1WithPassword 是本函数的导出名
// 参数:
//   - priKey: SM2 私钥(支持 PEM/HEX/原始字节，可为加密私钥)
//   - data: 待解密的 ASN.1 编码密文字节
//   - password: 私钥保护密码，未加密时传 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 未加密私钥时 password 传 nil
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// ct = codec.Sm2EncryptAsn1(pub, "secret")~
// pt = codec.Sm2DecryptAsn1WithPassword(priv, ct, nil)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: secret
// // assert: 锁定结论(带密码接口在 nil 密码下也能解密)
// assert string(pt) == "secret", "SM2 ASN1 with password(nil) should recover plaintext"
// ```
func SM2DecryptASN1WithPassword(priKey []byte, data []byte, password []byte) ([]byte, error) {
	pri, err := parsePrivateKey(priKey, password)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.privateKey")
	}

	return sm2.DecryptAsn1(pri, data)
}

// SM2SignWithSM3 使用国密 SM2 私钥对数据进行 SM3 签名，返回 ASN.1 DER 编码的签名
// 参数:
//   - priKeyBytes: SM2 私钥(支持 PEM/HEX/32 字节原始字节)
//   - data: 待签名的数据，可为 string、[]byte 等
//
// 返回值:
//   - []byte: ASN.1 DER 编码的 SM2 签名(每次随机，结果不固定)
//   - error: 签名失败时返回的错误
//
// Example:
// ```
// // VARS: 生成密钥对，签名后用公钥验签
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// sig = codec.Sm2SignWithSM3(priv, "msg")~
// // STDOUT: 验签返回 error，nil 表示通过
// println(codec.Sm2VerifyWithSM3(pub, "msg", sig) == nil)   // OUT: true
// // assert: 锁定结论(签名可被对应公钥验证通过)
// assert codec.Sm2VerifyWithSM3(pub, "msg", sig) == nil, "SM2 sign should be verifiable"
// ```
func SM2SignWithSM3(priKeyBytes []byte, data interface{}) ([]byte, error) {
	return SM2SignWithSM3WithPassword(priKeyBytes, data, nil)
}

// SM2SignWithSM3WithPassword 使用带密码保护的国密 SM2 私钥对数据进行 SM3 签名
// 参数:
//   - priKeyBytes: SM2 私钥(支持 PEM/HEX/原始字节，可为加密私钥)
//   - data: 待签名的数据，可为 string、[]byte 等
//   - password: 私钥保护密码，未加密时传 nil
//
// 返回值:
//   - []byte: ASN.1 DER 编码的 SM2 签名(每次随机，结果不固定)
//   - error: 签名失败时返回的错误
//
// Example:
// ```
// // VARS: 未加密私钥时 password 传 nil
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// sig = codec.Sm2SignWithSM3WithPassword(priv, "msg", nil)~
// // STDOUT: 验签返回 error，nil 表示通过
// println(codec.Sm2VerifyWithSM3(pub, "msg", sig) == nil)   // OUT: true
// // assert: 锁定结论(带密码接口在 nil 密码下也能签名并验证)
// assert codec.Sm2VerifyWithSM3(pub, "msg", sig) == nil, "SM2 sign with password(nil) should be verifiable"
// ```
func SM2SignWithSM3WithPassword(priKeyBytes []byte, data interface{}, password []byte) ([]byte, error) {
	// 检查数据是否为nil，如果是则报错
	if data == nil {
		return nil, errors.New("data cannot be nil")
	}

	dataBytes := interfaceToBytes(data)

	pri, err := parsePrivateKey(priKeyBytes, password)
	if err != nil {
		return nil, errors.Wrap(err, "parse SM2 private key failed")
	}

	// 使用SM2标准签名接口，内部会计算SM3摘要
	signature, err := pri.Sign(cryptoRand.Reader, dataBytes, nil)
	if err != nil {
		return nil, errors.Wrap(err, "SM2 sign failed")
	}

	return signature, nil
}

// SM2VerifyWithSM3 使用国密 SM2 公钥对数据进行 SM3 签名验证，验证通过返回 nil
// 参数:
//   - pubKeyBytes: SM2 公钥(支持 PEM/HEX/64 或 65 字节原始字节)
//   - originData: 原始签名数据，可为 string、[]byte 等
//   - sign: 待验证的 ASN.1 DER 编码签名
//
// 返回值:
//   - error: 验证通过返回 nil，验证失败返回错误信息
//
// Example:
// ```
// // VARS: 生成密钥对并签名，再验签
// priv, pub = codec.Sm2GenerateHexKeyPair()~
// sig = codec.Sm2SignWithSM3(priv, "msg")~
// // STDOUT: 验签返回 error，nil 表示通过
// println(codec.Sm2VerifyWithSM3(pub, "msg", sig) == nil)   // OUT: true
// // assert: 锁定结论(正确签名验证通过)
// assert codec.Sm2VerifyWithSM3(pub, "msg", sig) == nil, "SM2 verify should pass for valid signature"
// ```
func SM2VerifyWithSM3(pubKeyBytes []byte, originData interface{}, sign []byte) error {
	// 检查数据是否为nil，如果是则报错
	if originData == nil {
		return errors.New("data cannot be nil")
	}

	dataBytes := interfaceToBytes(originData)

	pub, err := parsePublicKey(pubKeyBytes)
	if err != nil {
		return errors.Wrap(err, "parse SM2 public key failed")
	}

	// 使用SM2标准验证接口
	if pub.Verify(dataBytes, sign) {
		return nil
	}

	return errors.New("SM2 signature verification failed")
}

// SM2KeyExchange 执行SM2密钥交换算法
//
// 参数说明：
//   - keyLength: 期望的共享密钥长度（字节）
//   - idA: A方标识（[]byte）
//   - idB: B方标识（[]byte）
//   - priKey: 调用方私钥（[]byte，支持PEM、HEX、原始字节）
//   - pubKey: 对方公钥（[]byte，支持PEM、HEX、原始字节）
//   - tempPriKey: 调用方临时私钥（[]byte，支持PEM、HEX、原始字节）
//   - tempPubKey: 对方临时公钥（[]byte，支持PEM、HEX、原始字节）
//   - thisIsA: 如果是A方调用设置为true，B方调用设置为false
//
// 返回值：
//   - sharedKey: 协商得到的共享密钥（[]byte）
//   - s1: 验证值S1，用于A验证B的身份（[]byte）
//   - s2: 验证值S2，用于B验证A的身份（[]byte）
//   - error: 错误信息
//
// Example:
// ```
// // A方和B方各自生成长期密钥对
// priKeyA, pubKeyA, _ := codec.Sm2GenerateHexKeyPair()
// priKeyB, pubKeyB, _ := codec.Sm2GenerateHexKeyPair()
//
// // A方和B方各自生成临时密钥对
// tempPriKeyA, tempPubKeyA, _ := codec.Sm2GenerateHexKeyPair()
// tempPriKeyB, tempPubKeyB, _ := codec.Sm2GenerateHexKeyPair()
//
// // A方执行密钥交换
// sharedKeyA, s1A, s2A, err := codec.Sm2KeyExchange(32, []byte("Alice"), []byte("Bob"),
//
//	priKeyA, pubKeyB, tempPriKeyA, tempPubKeyB, true)
//
// die(err)
//
// // B方执行密钥交换
// sharedKeyB, s1B, s2B, err := codec.Sm2KeyExchange(32, []byte("Alice"), []byte("Bob"),
//
//	priKeyB, pubKeyA, tempPriKeyB, tempPubKeyA, false)
//
// die(err)
//
// println("A方协商密钥:", codec.EncodeToHex(sharedKeyA))
// println("B方协商密钥:", codec.EncodeToHex(sharedKeyB))
// ```
func SM2KeyExchange(keyLength int, idA, idB, priKey, pubKey, tempPriKey, tempPubKey []byte, thisIsA bool) ([]byte, []byte, []byte, error) {
	// 解析私钥
	pri, err := parsePrivateKey(priKey, nil)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "parse private key failed")
	}

	// 解析对方公钥
	pub, err := parsePublicKey(pubKey)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "parse peer public key failed")
	}

	// 解析临时私钥
	tempPri, err := parsePrivateKey(tempPriKey, nil)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "parse temporary private key failed")
	}

	// 解析对方临时公钥
	tempPub, err := parsePublicKey(tempPubKey)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "parse peer temporary public key failed")
	}

	// 调用底层密钥交换函数
	sharedKey, s1, s2, err := sm2.KeyExchange(keyLength, idA, idB, pri, pub, tempPri, tempPub, thisIsA)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "SM2 key exchange failed")
	}

	return sharedKey, s1, s2, nil
}

// SM2GenerateTemporaryKeyPair 生成用于密钥交换的临时密钥对
//
// 返回值：
//   - []byte: 临时私钥（HEX格式）
//   - []byte: 临时公钥（HEX格式）
//   - error: 错误信息
//
// Example:
// ```
// tempPriKey, tempPubKey, err := codec.Sm2GenerateTemporaryKeyPair()
// die(err)
// println("临时私钥:", string(tempPriKey))
// println("临时公钥:", string(tempPubKey))
// ```
func SM2GenerateTemporaryKeyPair() ([]byte, []byte, error) {
	return GenerateSM2PrivateKeyHEX()
}

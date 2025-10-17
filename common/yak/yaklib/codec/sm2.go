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

func SM2DecryptC1C2C3(priKey []byte, data []byte) ([]byte, error) {
	return SM2DecryptC1C2C3WithPassword(priKey, data, nil)
}

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

func SM2DecryptC1C3C2(priKey []byte, data []byte) ([]byte, error) {
	return SM2DecryptC1C3C2WithPassword(priKey, data, nil)
}

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

func SM2EncryptASN1(pubKey []byte, data []byte) ([]byte, error) {
	pub, err := parsePublicKey(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.publicKey")
	}

	return sm2.EncryptAsn1(pub, data, cryptoRand.Reader)
}

func SM2DecryptASN1(priKey []byte, data []byte) ([]byte, error) {
	return SM2DecryptASN1WithPassword(priKey, data, nil)
}

func SM2DecryptASN1WithPassword(priKey []byte, data []byte, password []byte) ([]byte, error) {
	pri, err := parsePrivateKey(priKey, password)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.privateKey")
	}

	return sm2.DecryptAsn1(pri, data)
}

// SM2SignWithSM3 使用SM2私钥对数据进行SM3签名，返回签名与错误
//
// 参数 priKeyBytes 表示 SM2 私钥，支持以下格式：
//   - PEM 编码（例如 "-----BEGIN PRIVATE KEY-----" 块）
//   - HEX 字符串格式（64位十六进制字符串）
//   - 原始字节数组（32字节的私钥数据）
//
// 参数 data 是要签名的原始数据，可以是 []byte、string 或其他可转换为字节数组的类型。
// 返回值是SM2签名结果（ASN.1 DER编码），如果签名失败则返回错误。
//
// Example:
// ```
// priKey, pubKey, _ := codec.Sm2GeneratePemKeyPair()
// data := "hello world"
// signature, err := codec.Sm2SignWithSM3(priKey, data)
// die(err)
// println("签名成功")
// ```
func SM2SignWithSM3(priKeyBytes []byte, data interface{}) ([]byte, error) {
	return SM2SignWithSM3WithPassword(priKeyBytes, data, nil)
}

// SM2SignWithSM3WithPassword 使用带密码保护的SM2私钥对数据进行SM3签名
//
// 参数 priKeyBytes 表示加密的 SM2 私钥（PEM格式）
// 参数 data 是要签名的原始数据
// 参数 password 是私钥的保护密码，如果私钥未加密则传入 nil
// 返回值是SM2签名结果（ASN.1 DER编码），如果签名失败则返回错误。
//
// Example:
// ```
// encryptedPriKey := []byte(`-----BEGIN ENCRYPTED PRIVATE KEY-----
// MIGHAgEAMBMGByqGSM49AgEGCCqBHM9VAYItBG0wawIBAQQg...
// -----END ENCRYPTED PRIVATE KEY-----`)
// data := "hello world"
// password := []byte("mypassword")
// signature, err := codec.Sm2SignWithSM3WithPassword(encryptedPriKey, data, password)
// die(err)
// println("加密私钥签名成功")
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

// SM2VerifyWithSM3 使用SM2公钥对数据进行SM3签名验证，返回错误
//
// 参数 pubKeyBytes 表示 SM2 公钥，支持以下格式：
//   - PEM 编码（例如 "-----BEGIN PUBLIC KEY-----" 块）
//   - HEX 字符串格式（128位或130位十六进制字符串）
//   - 原始字节数组（64或65字节的公钥数据）
//
// 参数 originData 是原始签名数据
// 参数 sign 是SM2签名结果（ASN.1 DER编码）
// 如果验证成功返回 nil，验证失败返回错误信息。
//
// Example:
// ```
// priKey, pubKey, _ := codec.Sm2GeneratePemKeyPair()
// data := "hello world"
// signature, _ := codec.Sm2SignWithSM3(priKey, data)
// err := codec.Sm2VerifyWithSM3(pubKey, data, signature)
//
//	if err == nil {
//	   println("签名验证成功")
//	}else {
//
//	   println("签名验证失败:", err.Error())
//	}
//
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

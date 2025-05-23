package tlsutils

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"hash"
	"strings"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func Encrypt(raw []byte, pemBytes []byte) (string, error) {
	b, _ := pem.Decode(pemBytes)
	pub, err := ParseRsaPublicKeyFromPemBlock(b)
	if err != nil {
		return "", utils.Errorf("parse public key failed: %s", err)
	}

	var enc []string
	subs, err := SplitBlock(raw, (pub.Size()-11)/2)
	if err != nil {
		return "", utils.Errorf("split block failed: %s", err)
	}
	for _, sub := range subs {
		rs, err := rsa.EncryptPKCS1v15(cryptorand.Reader, pub, []byte(sub))
		if err != nil {
			return "", utils.Errorf("enc sub[%s] failed: %s", sub, err)
		}
		enc = append(enc, codec.EncodeToHex(rs))
	}
	return strings.Join(enc, "\n"), nil
}

func Decrypt(r string, priPem []byte) ([]byte, error) {
	b, _ := pem.Decode(priPem)
	pri, err := x509.ParsePKCS1PrivateKey(b.Bytes)
	if err != nil {
		return nil, utils.Errorf("parse public key failed: %s", err)
	}

	var groups []string
	for line := range utils.ParseLines(r) {
		lRaw, err := codec.DecodeHex(line)
		if err != nil {
			return nil, utils.Errorf("parse hex failed: %s", err)
		}

		res, err := rsa.DecryptPKCS1v15(cryptorand.Reader, pri, lRaw)
		if err != nil {
			return nil, utils.Errorf("dec block[%s] failed: %s", codec.StrConvQuoteHex(string(lRaw)), err)
		}

		groups = append(groups, string(res))
	}

	return MergeBlock(groups)
}

func GeneratePrivateAndPublicKeyPEM() (pri []byte, pub []byte, _ error) {
	return GeneratePrivateAndPublicKeyPEMWithPrivateFormatter("pkcs#1")
}

func GeneratePrivateAndPublicKeyPEMWithPrivateFormatter(t string) (pri []byte, pub []byte, _ error) {
	return GeneratePrivateAndPublicKeyPEMWithPrivateFormatterWithSize(t, 2048)
}

func GeneratePrivateAndPublicKeyPEMWithPrivateFormatterWithSize(t string, size int) (pri []byte, pub []byte, _ error) {
	pk, err := rsa.GenerateKey(cryptorand.Reader, size)
	if err != nil {
		return
	}

	var priBuffer bytes.Buffer
	var priDer []byte
	switch strings.ToLower(t) {
	case "pkcs1", "pkcs#1":
		priDer = x509.MarshalPKCS1PrivateKey(pk)
	case "ec", "ecdsa":
		pk, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
		if err != nil {
			return nil, nil, err
		}
		priDer, err = x509.MarshalECPrivateKey(pk)
		if err != nil {
			return nil, nil, utils.Errorf("marshal ecdsa prikey failed: %s", err)
		}
	case "pkcs8", "pkcs#8":
		fallthrough
	default:
		priDer, err = x509.MarshalPKCS8PrivateKey(pk)
		if err != nil {
			return nil, nil, utils.Errorf("marshal pkcs8 prikey failed: %s", err)
		}
	}
	err = pem.Encode(&priBuffer, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: priDer,
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal prikey failed: %s", err)
	}

	var pubBuffer bytes.Buffer
	pubDir, err := x509.MarshalPKIXPublicKey(pk.Public())
	if err != nil {
		return nil, nil, utils.Errorf("marshal pubkey failed: %s", err)
	}
	err = pem.Encode(&pubBuffer, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDir,
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal pem pubkey failed: %s", err)
	}

	return priBuffer.Bytes(), pubBuffer.Bytes(), nil
}

func PemPkcs1v15Encrypt(pemBytes []byte, data interface{}) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.Wrap(errors.New("empty pem block"), "pem decode public key failed")
	}

	pub, err := ParseRsaPublicKeyFromPemBlock(block)
	if err != nil {
		return nil, errors.Wrap(err, `x509.ParsePKIXPublicKey(block.Bytes) failed`)
	}
	_, _ = dataBytes, pub

	results, err := rsa.EncryptPKCS1v15(cryptorand.Reader, pub, dataBytes)
	if err != nil {
		return nil, errors.Wrap(err, `rsa.EncryptPKCS1v15(cryptorand.Reader, pubKey, dataBytes) error`)
	}
	return results, err
}

// EncryptWithPkcs1v15/RSAEncryptWithPKCS1v15 使用 RSA 公钥和 PKCS#1 v1.5 填充方式对给定数据进行加密。
//
// 参数 raw 表示 RSA 公钥，支持以下格式：
//   - DER 编码的公钥（raw ASN.1 DER 字节流）
//   - Base64 编码的 DER 格式（自动解码）
//   - PEM 编码（例如 "-----BEGIN PUBLIC KEY-----" 或 "-----BEGIN RSA PUBLIC KEY-----" 块）
//   - Base64 编码的 PEM 格式（自动解码）
//
// 参数 data 是要加密的明文数据，可以是 []byte、string 或其他可转换为字节数组的类型。
// 返回值是加密后的密文（字节切片），如果加密失败则返回错误。
//
// Example:
// ```
//
//		raw := `
//		-----BEGIN PUBLIC KEY-----
//		MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAn...（略）
//		-----END PUBLIC KEY-----
//		`
//		ciphertext, err := EncryptWithPkcs1v15(raw, "hello world")
//	 ciphertext, err := RSAEncryptWithPKCS1v15(raw, "hello world")
//
// ```
func Pkcs1v15Encrypt(raw []byte, data interface{}) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	pub, err := GetRSAPubKey(raw)
	if err != nil {
		return nil, errors.Wrap(err, `GetRSAPubKey failed`)
	}
	_, _ = dataBytes, pub

	results, err := rsa.EncryptPKCS1v15(cryptorand.Reader, pub, dataBytes)
	if err != nil {
		return nil, errors.Wrap(err, `rsa.EncryptPKCS1v15(cryptorand.Reader, pubKey, dataBytes) error`)
	}
	return results, err
}

func ParseRsaPublicKeyFromPemBlock(block *pem.Block) (*rsa.PublicKey, error) {
	derBytes := block.Bytes
	var pub *rsa.PublicKey
	var key any
	var err error
	key, err = x509.ParsePKIXPublicKey(derBytes)
	if err != nil {
		key, err = x509.ParsePKCS1PublicKey(derBytes)
		if err != nil {
			return nil, errors.New("derBytes from pem block is neither PKIXPublicKey nor PKCS1PublicKey")
		}
	}
	pub, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("need *rsa.PublicKey, got %t", key)
	}
	return pub, nil
}

func ParseRsaPublicKeyFromDerBytes(derBytes []byte) (*rsa.PublicKey, error) {
	var pub *rsa.PublicKey
	var key any
	var err error
	key, err = x509.ParsePKIXPublicKey(derBytes)
	if err != nil {
		key, err = x509.ParsePKCS1PublicKey(derBytes)
		if err != nil {
			return nil, errors.New("derBytes is neither PKIXPublicKey nor PKCS1PublicKey")
		}
	}
	pub, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("need *rsa.PublicKey, got %t", key)
	}
	return pub, nil
}

func GetRSAPubKey(raw []byte) (*rsa.PublicKey, error) {
	var err error
	var decodedBytes []byte
	var pub *rsa.PublicKey

	block, _ := pem.Decode(raw) // check for raw pem

	if block == nil { // check for base64 encoded pem
		decodedBytes, err = codec.DecodeBase64(string(raw))
		if err == nil {
			block, _ = pem.Decode(decodedBytes)
		}
	}

	if block == nil { // raw is not pem format
		rawString := strings.ReplaceAll(strings.TrimSpace(string(raw)), "\n", "")
		decodedBytes, err = codec.DecodeBase64(rawString)
		if err != nil {
			pub, err = ParseRsaPublicKeyFromDerBytes(raw)
			if err != nil {
				return nil, errors.New("all strategies failed to parse public key")
			}
			return pub, nil
		}
		return ParseRsaPublicKeyFromDerBytes(decodedBytes)
	}
	return ParseRsaPublicKeyFromPemBlock(block)
}

func ParseRsaPrivateKeyFromPemBlock(block *pem.Block) (*rsa.PrivateKey, error) {
	derBytes := block.Bytes
	var pri *rsa.PrivateKey
	var err error
	pri, err = x509.ParsePKCS1PrivateKey(derBytes)
	if err != nil {
		parsedPri, err := x509.ParsePKCS8PrivateKey(derBytes)
		if err != nil {
			return nil, utils.Errorf("parse private key failed: %s", err)
		}
		var ok bool
		pri, ok = parsedPri.(*rsa.PrivateKey)
		if !ok {
			return nil, utils.Errorf("need *rsa.PrivateKey, got %t", parsedPri)
		}
	}
	return pri, nil
}

func ParseRsaPrivateKeyFromDerBytes(derBytes []byte) (*rsa.PrivateKey, error) {
	var pri *rsa.PrivateKey
	var err error
	pri, err = x509.ParsePKCS1PrivateKey(derBytes)
	if err != nil {
		parsedPri, err := x509.ParsePKCS8PrivateKey(derBytes)
		if err != nil {
			return nil, utils.Errorf("parse private key failed: %s", err)
		}
		var ok bool
		pri, ok = parsedPri.(*rsa.PrivateKey)
		if !ok {
			return nil, utils.Errorf("need *rsa.PrivateKey, got %t", parsedPri)
		}
	}
	return pri, nil
}

func GetRSAPrivateKey(raw []byte) (*rsa.PrivateKey, error) {
	var err error
	var decodedBytes []byte
	var pri *rsa.PrivateKey

	block, _ := pem.Decode(raw) // check for raw pem

	if block == nil { // check for base64 encoded pem
		decodedBytes, err = codec.DecodeBase64(string(raw))
		if err == nil {
			block, _ = pem.Decode(decodedBytes)
		}
	}

	if block == nil {
		rawString := strings.ReplaceAll(strings.TrimSpace(string(raw)), "\n", "")
		decodedBytes, err = codec.DecodeBase64(rawString)
		if err != nil {
			pri, err = ParseRsaPrivateKeyFromDerBytes(raw)
			if err != nil {
				return nil, errors.New("all strategies failed to parse private key")
			}
			return pri, nil
		}
		return ParseRsaPrivateKeyFromDerBytes(decodedBytes)
	}
	return ParseRsaPrivateKeyFromPemBlock(block)
}

func PemPkcsOAEPEncrypt(raw []byte, data interface{}) ([]byte, error) {
	return PkcsOAEPEncryptWithHash(raw, data, sha256.New())
}

// RSAEncryptWithOAEP 使用 RSA 公钥和 OAEP 填充方式对给定数据进行加密。
//
// 参数 raw 表示 RSA 公钥，支持以下格式：
//   - DER 编码的公钥（raw ASN.1 DER 字节流）
//   - Base64 编码的 DER 格式（自动解码）
//   - PEM 编码（例如 "-----BEGIN PUBLIC KEY-----" 或 "-----BEGIN RSA PUBLIC KEY-----" 块）
//   - Base64 编码的 PEM 格式（自动解码）
//
// 参数 data 是要加密的明文数据，可以是 []byte、string 或其他可转换为字节数组的类型。
// 返回值是加密后的密文（字节切片），如果加密失败则返回错误。
//
// Example:
// ```
//
//	raw := `
//	-----BEGIN PUBLIC KEY-----
//	MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAn...（略）
//	-----END PUBLIC KEY-----
//	`
//	ciphertext, err := RSAEncryptWithOAEP(raw, "hello world")
//
// ```
func PkcsOAEPEncrypt(raw []byte, data interface{}) ([]byte, error) {
	return PkcsOAEPEncryptWithHash(raw, data, sha256.New())
}

func PkcsOAEPEncryptWithHash(raw []byte, data interface{}, hashFunc hash.Hash) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	pub, err := GetRSAPubKey(raw)
	if err != nil {
		return nil, errors.Wrap(err, `GetRSAPubKey failed`)
	}
	_, _ = dataBytes, pub

	results, err := rsa.EncryptOAEP(hashFunc, cryptorand.Reader, pub, dataBytes, nil)
	if err != nil {
		return nil, errors.Wrap(err, `rsa.EncryptOAEP(cryptorand.Reader, pubKey, dataBytes) error`)
	}
	return results, err
}

func PemPkcsOAEPDecrypt(pemPriBytes []byte, data interface{}) ([]byte, error) {
	return PkcsOAEPDecryptWithHash(pemPriBytes, data, sha256.New())
}

// RSADecryptWithOAEP 使用 RSA私钥 和 RSA-OAEP 填充方式解密给定的密文。
// 参数 raw 表示 RSA 私钥，支持以下格式：
//   - DER 编码的私钥（raw ASN.1 DER 字节流）
//   - Base64 编码的 DER 格式（自动解码）
//   - PEM 编码（包括带有 "-----BEGIN PRIVATE KEY-----" 或 "-----BEGIN RSA PRIVATE KEY-----" 的块）
//   - Base64 编码的 PEM 格式（自动解码）
//
// 参数 data 是加密后的数据（密文），可以是 []byte 或 base64 字符串等支持类型。
// 返回值是解密得到的原始明文，如果失败则返回错误。
//
// 示例：
//
//	raw := `
//	-----BEGIN PRIVATE KEY-----
//	MIIEvQIBADANBgkqhkiG9w0BAQEFAASC...（略）
//	-----END PRIVATE KEY-----
//	`
//	plaintext, err := Pkcs1v15Decrypt([]byte(raw), encryptedData)
func PkcsOAEPDecrypt(raw []byte, data interface{}) ([]byte, error) {
	return PkcsOAEPDecryptWithHash(raw, data, sha256.New())
}

func PkcsOAEPDecryptWithHash(raw []byte, data interface{}, hashFunc hash.Hash) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	pri, err := GetRSAPrivateKey(raw)
	if err != nil {
		return nil, errors.Wrap(err, `GetRSAPrivateKey failed`)
	}

	results, err := rsa.DecryptOAEP(hashFunc, cryptorand.Reader, pri, dataBytes, nil)
	if err != nil {
		return nil, errors.Wrap(err, `rsa.PemPkcsOAEPDecrypt(cryptorand.Reader, pri, dataBytes) error`)
	}
	return results, err
}

func PemPkcs1v15Decrypt(pemPriBytes []byte, data interface{}) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	b, _ := pem.Decode(pemPriBytes)
	pri, err := x509.ParsePKCS1PrivateKey(b.Bytes)
	if err != nil {
		parsedPri, err := x509.ParsePKCS8PrivateKey(b.Bytes)
		if err != nil {
			return nil, utils.Errorf("parse private key failed: %s", err)
		}

		var ok bool
		pri, ok = parsedPri.(*rsa.PrivateKey)
		if !ok {
			return nil, utils.Errorf("need *rsa.PrivateKey, cannot found! ")
		}
	}

	results, err := rsa.DecryptPKCS1v15(cryptorand.Reader, pri, dataBytes)
	if err != nil {
		return nil, errors.Wrap(err, `rsa.DecryptPKCS1v15(cryptorand.Reader, pri, dataBytes) error`)
	}
	return results, err
}

// DecryptWithPkcs1v15/RSADecryptWithPKCS1v15 使用 RSA私钥 和 PKCS#1 v1.5填充方式 解密给定的密文。
// 参数 raw 表示 RSA 私钥，支持以下格式：
//   - DER 编码的私钥（raw ASN.1 DER 字节流）
//   - Base64 编码的 DER 格式（自动解码）
//   - PEM 编码（包括带有 "-----BEGIN PRIVATE KEY-----" 或 "-----BEGIN RSA PRIVATE KEY-----" 的块）
//   - Base64 编码的 PEM 格式（自动解码）
//
// 参数 data 是被加密后的数据（密文）
// 返回值是解密得到的原始明文，如果失败则返回错误。
//
// Example:
// ```
//
//		raw := `
//		-----BEGIN PRIVATE KEY-----
//		MIIEvQIBADANBgkqhkiG9w0BAQEFAASC...（略）
//		-----END PRIVATE KEY-----
//		`
//		plaintext, err := DecryptWithPkcs1v15(raw, encryptedData)
//	 	plaintext, err := RSADecryptWithPKCS1v15(raw, encryptedData)
//
// ```
func Pkcs1v15Decrypt(raw []byte, data interface{}) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	pri, err := GetRSAPrivateKey(raw)
	if err != nil {
		return nil, errors.Wrap(err, `GetRSAPrivateKey failed`)
	}

	results, err := rsa.DecryptPKCS1v15(cryptorand.Reader, pri, dataBytes)
	if err != nil {
		return nil, errors.Wrap(err, `rsa.DecryptPKCS1v15(cryptorand.Reader, pri, dataBytes) error`)
	}
	return results, err
}

// SignSHA256WithRSA 使用RSA私钥对数据进行SHA256签名，返回签名与错误
// Example:
// ```
// pemBytes = string(`-----BEGIN PRIVATE KEY-----
// MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDZz5Zz3z3z3z3z
// ...
// -----END PRIVATE KEY-----`)
// signBytes, err := tls.SignSHA256WithRSA(pemBytes, "hello")
// die(err)
// signString = string(signBytes)
// ```
func PemSignSha256WithRSA(pemBytes []byte, data interface{}) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	block, _ := pem.Decode(pemBytes)
	pri, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		pri, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, utils.Errorf("parse private (PKCS8 / PKCS1) key failed: %s", err)
		}
	}
	pkey, ok := pri.(*rsa.PrivateKey)
	if !ok {
		return nil, utils.Errorf("need *rsa.PrivateKey, cannot found! but got: %T", pri)
	}
	var results []byte = make([]byte, 32)
	for i, v := range sha256.Sum256(dataBytes) {
		results[i] = v
	}
	return rsa.SignPKCS1v15(cryptorand.Reader, pkey, crypto.SHA256, results)
}

// SignVerifySHA256WithRSA 使用RSA公钥对数据进行SHA256签名验证，返回错误
// Example:
// ```
// pemBytes = string(`-----BEGIN PUBLIC KEY-----
// MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAs1pvFYNQpPSPbshg6F7Z
// ...
// -----END PUBLIC KEY-----`)
// err := tls.PemVerifySignSha256WithRSA(pemBytes, "hello", signBytes)
// die(err)
// ```
func PemVerifySignSha256WithRSA(pemBytes []byte, originData any, sign []byte) error {
	dataBytes := utils.InterfaceToBytes(originData)
	block, _ := pem.Decode(pemBytes)

	pub, err := ParseRsaPublicKeyFromPemBlock(block)
	if err != nil {
		return utils.Errorf("parse public key failed: %s", err)
	}
	var origin []byte = make([]byte, 32)
	for i, v := range sha256.Sum256(dataBytes) {
		origin[i] = v
	}
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, origin, sign)
}

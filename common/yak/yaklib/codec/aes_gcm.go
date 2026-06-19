package codec

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"io"

	"github.com/pkg/errors"
)

/*
//AES GCM 加密后的payload shiro 1.4.2版本更换为了AES-GCM加密方式

	func AES_GCM_Encrypt(key []byte, Content []byte) string {
		block, _ := aes.NewCipher(key)
		nonce := make([]byte, 16)
		io.ReadFull(rand.Reader, nonce)
		aesgcm, _ := cipher.NewGCMWithNonceSize(block, 16)
		ciphertext := aesgcm.Seal(nil, nonce, Content, nil)
		return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...))
	}
*/
// AESGCMEncrypt 使用 AES-GCM 认证加密模式加密数据；nonceRaw 为空时随机生成 nonce 并前置到密文中
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)；nonce 长度为 16 时用 16，否则用 12。
// 注意：AESGCMEncryptWithNonceSize16 是本函数的别名
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - data: 待加密的数据，可为 string、[]byte 等
//   - nonceRaw: nonce(随机数)，传 nil 则自动生成并前置到密文
//
// 返回值:
//   - []byte: 加密后的密文字节(随机 nonce 时前 nonceSize 字节为 nonce)
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: nonce 传 nil 自动生成并前置
// key = "1234567890123456"
// ct = codec.AESGCMEncrypt(key, "Secret Message", nil)~
// pt = codec.AESGCMDecrypt(key, ct, nil)~
// // STDOUT: 打印解密还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(GCM 加解密往返一致)
// assert string(pt) == "Secret Message", "AES-GCM encrypt/decrypt should round-trip"
// ```
func AESGCMEncrypt(key []byte, data interface{}, nonceRaw []byte) ([]byte, error) {
	nonceSize := 12
	if len(nonceRaw) == 16 {
		nonceSize = 16
	}
	return AESGCMEncryptWithNonceSize(key, data, nonceRaw, nonceSize)
}

var AESGCMEncryptWithNonceSize16 = AESGCMEncrypt

// AESGCMEncryptWithNonceSize12 使用 AES-GCM 模式以 12 字节 nonce 加密数据
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - data: 待加密的数据，可为 string、[]byte 等
//   - nonceRaw: nonce(随机数)，传 nil 则自动生成 12 字节并前置到密文
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: nonce 传 nil 自动生成(12 字节)并前置
// key = "1234567890123456"
// ct = codec.AESGCMEncryptWithNonceSize12(key, "Secret Message", nil)~
// pt = codec.AESGCMDecryptWithNonceSize12(key, ct, nil)~
// // STDOUT: 打印解密还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(GCM nonce12 加解密往返一致)
// assert string(pt) == "Secret Message", "AES-GCM nonce12 should round-trip"
// ```
func AESGCMEncryptWithNonceSize12(key []byte, data interface{}, nonceRaw []byte) ([]byte, error) {
	return AESGCMEncryptWithNonceSize(key, data, nonceRaw, 12)
}

func AESGCMEncryptWithNonceSize(key []byte, data interface{}, nonceRaw []byte, nonceSize int) ([]byte, error) {
	dataRaw := interfaceToBytes(data)

	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Errorf("create aes cipher failed: %s", err)
	}

	gcm, err := cipher.NewGCMWithNonceSize(c, nonceSize)
	if err != nil {
		return nil, errors.Errorf("create gcm failed: %s", err)
	}

	nonce := make([]byte, gcm.NonceSize())

	var randomNonce bool
	if len(nonceRaw) > 0 {
		copy(nonce, nonceRaw)
	} else {
		if _, err := io.ReadFull(cryptorand.Reader, nonce); err != nil {
			return nil, errors.Errorf("read nonce for aes_gcm failed: %s", err)
		}
		randomNonce = true
	}

	var buf bytes.Buffer
	if randomNonce {
		buf.Write(nonce)
	}
	buf.Write(gcm.Seal(nil, nonce, dataRaw, nil))
	return buf.Bytes(), nil
}

func AESGCMDecryptWithNonceSize(key []byte, data interface{}, nonceRaw []byte, nonceSize int) ([]byte, error) {
	dataRaw := interfaceToBytes(data)

	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Errorf("create aes cipher failed: %s", err)
	}

	gcm, err := cipher.NewGCMWithNonceSize(c, nonceSize)
	if err != nil {
		return nil, errors.Errorf("create gcm failed: %s", err)
	}

	// 兼容 nonce
	var nonce = make([]byte, nonceSize)
	if nonceRaw != nil {
		copy(nonce, nonceRaw)
	} else {
		if len(dataRaw) < nonceSize {
			return nil, errors.Errorf("nonce is empty, data[%v] is too short(cannot found nonce), ", StrConvQuoteHex(string(dataRaw)))
		}

		nonceFromData, encryptedData := dataRaw[:nonceSize], dataRaw[nonceSize:]
		copy(nonce, nonceFromData)
		if plain, err := gcm.Open(nil, nonce, encryptedData, nil); err == nil {
			return plain, nil
		}
	}
	return gcm.Open(nil, nonce, dataRaw, nil)
}

// AESGCMDecryptWithNonceSize12 使用 AES-GCM 模式以 12 字节 nonce 解密数据
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - data: 待解密的密文，可为 []byte 等
//   - nonce: nonce(随机数)，传 nil 则从密文前 12 字节提取
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密或认证失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(nonce12)
// key = "1234567890123456"
// ct = codec.AESGCMEncryptWithNonceSize12(key, "Secret Message", nil)~
// pt = codec.AESGCMDecryptWithNonceSize12(key, ct, nil)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(nonce12 解密还原一致)
// assert string(pt) == "Secret Message", "AES-GCM nonce12 decrypt should recover plaintext"
// ```
func AESGCMDecryptWithNonceSize12(key []byte, data interface{}, nonce []byte) ([]byte, error) {
	return AESGCMDecryptWithNonceSize(key, data, nonce, 12)
}

// AESGCMDecryptWithNonceSize16 使用 AES-GCM 模式以 16 字节 nonce 解密数据
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - data: 待解密的密文，可为 []byte 等
//   - nonce: nonce(随机数)，传 nil 则从密文前 16 字节提取
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密或认证失败时返回的错误
//
// Example:
// ```
// // VARS: 先用 16 字节 nonce 加密再解密
// key = "1234567890123456"
// ct = codec.AESGCMEncrypt(key, "Secret Message", "0123456789abcdef")~
// pt = codec.AESGCMDecryptWithNonceSize16(key, ct, "0123456789abcdef")~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(nonce16 解密还原一致)
// assert string(pt) == "Secret Message", "AES-GCM nonce16 decrypt should recover plaintext"
// ```
func AESGCMDecryptWithNonceSize16(key []byte, data interface{}, nonce []byte) ([]byte, error) {
	return AESGCMDecryptWithNonceSize(key, data, nonce, 16)
}

// AESGCMDecrypt 使用 AES-GCM 认证加密模式解密数据；nonce 为空时从密文前置部分提取 nonce
// 密钥长度必须是 16/24/32 字节；nonce 长度为 16 时用 16，否则用 12。
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - data: 待解密的密文，可为 []byte 等
//   - nonce: nonce(随机数)，传 nil 则从密文前置部分提取
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密或认证失败时返回的错误
//
// Example:
// ```
// // VARS: nonce 传 nil，从密文中提取
// key = "1234567890123456"
// ct = codec.AESGCMEncrypt(key, "Secret Message", nil)~
// pt = codec.AESGCMDecrypt(key, ct, nil)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(GCM 解密还原一致)
// assert string(pt) == "Secret Message", "AES-GCM decrypt should recover plaintext"
// ```
func AESGCMDecrypt(key []byte, data interface{}, nonce []byte) ([]byte, error) {
	nonceSize := 12
	if len(nonce) == 16 {
		nonceSize = 16
	}
	return AESGCMDecryptWithNonceSize(key, data, nonce, nonceSize)
}

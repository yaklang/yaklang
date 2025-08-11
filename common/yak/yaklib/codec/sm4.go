package codec

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/gmsm/sm4"
)

//	func sm4encBase(data interface{}, key []byte, iv []byte, sm4ordinary func(key, in []byte, encode bool, iv []byte) ([]byte, error)) ([]byte, error) {
//		return sm4ordinary(key, interfaceToBytes(data), true, iv)
//	}
//
//	func sm4decBase(data interface{}, key []byte, iv []byte, sm4ordinary func(key, in []byte, encode bool, iv []byte) ([]byte, error)) ([]byte, error) {
//		return sm4ordinary(key, interfaceToBytes(data), false, iv)
//	}
//
//	func SM4CFBEnc(key []byte, data interface{}, iv []byte) ([]byte, error) {
//		return sm4encBase(data, key, iv, sm4.Sm4CFB)
//	}
//
//	func SM4CBCEnc(key []byte, data interface{}, iv []byte) ([]byte, error) {
//		return sm4encBase(data, key, iv, sm4.Sm4Cbc)
//	}
//
//	func SM4ECBEnc(key []byte, data interface{}, iv []byte) ([]byte, error) {
//		return sm4encBase(data, key, iv, sm4.Sm4Ecb)
//	}
//
//	func SM4OFBEnc(key []byte, data interface{}, iv []byte) ([]byte, error) {
//		return sm4encBase(data, key, iv, sm4.Sm4OFB)
//	}
//
//	func SM4CFBDec(key []byte, data interface{}, iv []byte) ([]byte, error) {
//		return sm4decBase(data, key, iv, sm4.Sm4CFB)
//	}
//
//	func SM4CBCDec(key []byte, data interface{}, iv []byte) ([]byte, error) {
//		return sm4decBase(data, key, iv, sm4.Sm4Cbc)
//	}
//
//	func SM4ECBDec(key []byte, data interface{}, iv []byte) ([]byte, error) {
//		return sm4decBase(data, key, iv, sm4.Sm4Ecb)
//	}
//
//	func SM4OFBDec(key []byte, data interface{}, iv []byte) ([]byte, error) {
//		return sm4decBase(data, key, iv, sm4.Sm4OFB)
//	}

// Sm4GCMEncrypt 使用 SM4 算法，在 GCM 模式下加密数据
// GCM 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// Example:
// ```
// codec.Sm4GCMEncrypt("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4GCMEnc(key []byte, data interface{}, iv []byte) ([]byte, error) {
	if iv == nil {
		iv = key[:sm4.BlockSize]
	}
	raw := sm4.PKCS7Padding(interfaceToBytes(data))
	result, _, err := sm4.Sm4GCM(key, iv, raw, nil, true)
	if err != nil {
		return nil, errors.Errorf("sm4 gcm enc failed: %s", err)
	}
	return result, nil
}

// Sm4GCMDecrypt 使用 SM4 算法，在 GCM 模式下解密数据
// GCM 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// Example:
// ```
// codec.Sm4GCMDecrypt("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4GCMDec(key []byte, data interface{}, iv []byte) ([]byte, error) {
	if iv == nil {
		iv = key[:sm4.BlockSize]
	}

	result, _, err := sm4.Sm4GCM(key, iv, interfaceToBytes(data), nil, false)
	if err != nil {
		return nil, errors.Errorf("sm4 gcm dec failed: %s", err)
	}
	return sm4.PKCS7UnPadding(result), nil
}

// Construct functions corresponding to various encryption modes, export func

// SM4EncryptCBCWithPKCSPadding 使用 SM4 算法，在 CBC 模式下，使用 PKCS#7 填充来加密数据
// CBC 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// 注意：SM4Encrypt SM4CBCEncrypt 和 SM4EncryptCBCWithPKCSPadding 是同一个函数的别名
// Example:
// ```
// codec.SM4EncryptCBCWithPKCSPadding("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4EncryptCBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, CBC)(key, i, iv)
}

// SM4DecryptCBCWithPKCSPadding 使用 SM4 算法，在 CBC 模式下，使用 PKCS#7 填充来解密数据
// CBC 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// 注意：SM4Decrypt SM4CBCDecrypt 和 SM4DecryptCBCWithPKCSPadding 是同一个函数的别名
// Example:
// ```
// codec.SM4DecryptCBCWithPKCSPadding("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4DecryptCBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, CBC)(key, i, iv)
}

// SM4EncryptECBWithPKCSPadding 使用 SM4 算法，在 ECB 模式下，使用 PKCS#7 填充来加密数据
// ECB 模式下不需要 IV (初始化向量)，因此其是一个无用字段。
// 注意：SM4ECBEncrypt 和 SM4EncryptECBWithPKCSPadding 是同一个函数的别名
// Example:
// ```
// codec.SM4EncryptECBWithPKCSPadding("1234123412341234", "123412341234123456", nil)
// ```
func SM4EncryptECBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, ECB)(key, i, iv)
}

// SM4DecryptECBWithPKCSPadding 使用 SM4 算法，在 ECB 模式下，使用 PKCS#7 填充来解密数据
// ECB 模式下不需要 IV (初始化向量)，因此其是一个无用字段。
// 注意：SM4ECBDecrypt 和 SM4DecryptECBWithPKCSPadding 是同一个函数的别名
// Example:
// ```
// codec.SM4DecryptECBWithPKCSPadding("1234123412341234", "123412341234123456", nil)
// ```
func SM4DecryptECBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, ECB)(key, i, iv)
}

// SM4EncryptECBWithPKCSPadding 使用 SM4 算法，在 ECB 模式下，使用 PKCS#7 填充来加密数据
// Deprecated: 请使用 Sm4ECBEncrypt（EBC 是 ECB 的拼写错误）
func SM4EncryptEBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, ECB)(key, i, iv)
}

// SM4DecryptECBWithPKCSPadding 使用 SM4 算法，在 ECB 模式下，使用 PKCS#7 填充来解密数据
// Deprecated: 请使用 Sm4ECBDecrypt（EBC 是 ECB 的拼写错误）
func SM4DecryptEBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, ECB)(key, i, iv)
}

// SM4EncryptCFBWithPKCSPadding 使用 SM4 算法，在 CFB 模式下，使用 PKCS#7 填充来加密数据
// CFB 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// 注意：SM4CFBEncrypt 和 SM4EncryptCFBWithPKCSPadding 是同一个函数的别名
// Example:
// ```
// codec.SM4EncryptCFBWithPKCSPadding("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4EncryptCFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, CFB)(key, i, iv)
}

// SM4DecryptCFBWithPKCSPadding 使用 SM4 算法，在 CFB 模式下，使用 PKCS#7 填充来解密数据
// CFB 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// 注意：SM4CFBDecrypt 和 SM4DecryptCFBWithPKCSPadding 是同一个函数的别名
// Example:
// ```
// codec.SM4DecryptCFBWithPKCSPadding("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4DecryptCFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, CFB)(key, i, iv)
}

// SM4EncryptOFBWithPKCSPadding 使用 SM4 算法，在 OFB 模式下，使用 PKCS#7 填充来加密数据
// OFB 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// 注意：SM4OFBEncrypt 和 SM4EncryptOFBWithPKCSPadding 是同一个函数的别名
// Example:
// ```
// codec.SM4EncryptOFBWithPKCSPadding("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4EncryptOFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, OFB)(key, i, iv)
}

// SM4DecryptOFBWithPKCSPadding 使用 SM4 算法，在 OFB 模式下，使用 PKCS#7 填充来解密数据
// OFB 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// 注意：SM4OFBDecrypt 和 SM4DecryptOFBWithPKCSPadding 是同一个函数的别名
// Example:
// ```
// codec.SM4DecryptOFBWithPKCSPadding("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4DecryptOFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, OFB)(key, i, iv)
}

// other func
var SM4EncryptCBCWithZeroPadding = SM4EncFactory(ZeroPadding, CBC)
var SM4DecryptCBCWithZeroPadding = SM4DecFactory(ZeroUnPadding, CBC)
var SM4EncryptCFBWithZeroPadding = SM4EncFactory(ZeroPadding, CFB)
var SM4DecryptCFBWithZeroPadding = SM4DecFactory(ZeroUnPadding, CFB)
var SM4EncryptECBWithZeroPadding = SM4EncFactory(ZeroPadding, ECB)
var SM4DecryptECBWithZeroPadding = SM4DecFactory(ZeroUnPadding, ECB)
var SM4EncryptOFBWithZeroPadding = SM4EncFactory(ZeroPadding, OFB)
var SM4DecryptOFBWithZeroPadding = SM4DecFactory(ZeroUnPadding, OFB)
var SM4EncryptCTRWithPKCSPadding = SM4EncFactory(PKCS5Padding, CTR)
var SM4DecryptCTRWithPKCSPadding = SM4DecFactory(PKCS5UnPadding, CTR)
var SM4EncryptCTRWithZeroPadding = SM4EncFactory(ZeroPadding, CTR)
var SM4DecryptCTRWithZeroPadding = SM4DecFactory(ZeroUnPadding, CTR)
var SM4GCMEncrypt = SM4GCMEnc
var SM4GCMDecrypt = SM4GCMDec

func SM4EncFactory(paddingFunc func([]byte, int) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		data := paddingFunc(interfaceToBytes(i), 16)
		iv = FixIV(iv, key, 16)
		return SM4Enc(key, data, iv, mode)
	}
}

func SM4DecFactory(unpaddingFunc func([]byte) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		iv = FixIV(iv, key, 16)
		res, err := SM4Dec(key, interfaceToBytes(i), iv, mode)
		if err != nil {
			return nil, err
		}
		return unpaddingFunc(res), nil
	}
}

func SM4Enc(key, data, iv []byte, mode string) ([]byte, error) {
	data = ZeroPadding(data, sm4.BlockSize)
	c, err := sm4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	switch mode {
	case CBC:
		return CBCEncode(c, iv, data)
	case ECB:
		return ECBEncode(c, data)
	case CFB:
		return CFBEncode(c, iv, data)
	case OFB:
		return OFBEncode(c, iv, data)
	case CTR:
		return CTREncode(c, iv, data)
	default:
		return nil, fmt.Errorf("SM4: invalid mode %s", mode)
	}
}

func SM4Dec(key, data, iv []byte, mode string) ([]byte, error) {
	c, err := sm4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	switch mode {
	case CBC:
		return CBCDecode(c, iv, data)
	case ECB:
		return ECBDecode(c, data)
	case CFB:
		return CFBDecode(c, iv, data)
	case OFB:
		return OFBDecode(c, iv, data)
	case CTR:
		return CTRDecode(c, iv, data)
	default:
		return nil, fmt.Errorf("SM4: invalid mode %s", mode)
	}
}

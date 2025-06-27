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

// Sm4CBCEncrypt 使用 SM4 算法，在 CBC 模式下，使用 PKCS#5 填充来加密数据
// CBC 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// example:
// ```
// codec.Sm4CBCEncrypt("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4EncryptCBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, CBC)(key, i, iv)
}

// Sm4CBCDecrypt 使用 SM4 算法，在 CBC 模式下，使用 PKCS#5 填充来解密数据
// CBC 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// Example:
// ```
// codec.Sm4CBCDecrypt("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4DecryptCBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, CBC)(key, i, iv)
}

// Sm4ECBEncrypt 使用 SM4 算法，在 ECB 模式下，使用 PKCS#5 填充来加密数据
// ECB 模式下不需要 IV (初始化向量)，因此其是一个无用字段.
// Example:
// ```
// codec.Sm4ECBEncrypt("1234123412341234", "123412341234123456", nil)
// ```
func SM4EncryptECBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, ECB)(key, i, iv)
}

// Sm4ECBDecrypt 使用 SM4 算法，在 ECB 模式下，使用 PKCS#5 填充来解密数据
// ECB 模式下不需要 IV (初始化向量)，因此其是一个无用字段。
// Example:
// ```
// codec.Sm4ECBDecrypt("1234123412341234", "123412341234123456", nil)
// ```
func SM4DecryptECBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, ECB)(key, i, iv)
}

// Sm4CFBEncrypt 使用 SM4 算法，在 CFB 模式下，使用 PKCS#5 填充来加密数据
// CFB 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// Example:
// ```
// codec.Sm4CFBEncrypt("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4EncryptCFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, CFB)(key, i, iv)
}

// Sm4CFBDecrypt 使用 SM4 算法，在 ECB 模式下，使用 PKCS#5 填充来解密数据
// ECB 模式下不需要 IV (初始化向量)，因此它可以为 nil (空)。
// Example:
// ```
// codec.Sm4CFBDecrypt("1234123412341234", "123412341234123456", nil)
// ```
func SM4DecryptCFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, CFB)(key, i, iv)
}

// Sm4OFBEncrypt 使用 SM4 算法，在 OFB 模式下，使用 PKCS#5 填充来加密数据
// OFB 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// Example:
// ```
// codec.Sm4OFBEncrypt("1234123412341234", "123412341234123456", "1234123412341234")
// ```
func SM4EncryptOFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, OFB)(key, i, iv)
}

// Sm4OFBDecrypt 使用 SM4 算法，在 OFB 模式下，使用 PKCS#5 填充来解密数据
// OFB 模式下需要 IV (初始化向量)，若为空则会使用 key 的前 16 字节作为 IV。
// Example:
// ```
// codec.Sm4OFBDecrypt("1234123412341234", "123412341234123456", "1234123412341234")
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

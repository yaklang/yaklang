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

// SM4GCMEnc 使用国密 SM4 算法在 GCM 模式下加密数据
// 密钥与 IV 均为 16 字节；IV 为空时使用 key 前 16 字节作为 IV。
// 参数:
//   - key: 密钥(16 字节)
//   - data: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: SM4-GCM 加解密
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.Sm4GCMEncrypt(key, "Secret Message", iv)~
// // STDOUT: 解密还原后打印
// println(string(codec.Sm4GCMDecrypt(key, ct, iv)~))   // OUT: Secret Message
// // assert: 锁定结论(SM4-GCM 加解密往返一致)
// assert string(codec.Sm4GCMDecrypt(key, ct, iv)~) == "Secret Message", "SM4-GCM should round-trip"
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

// SM4GCMDec 使用国密 SM4 算法在 GCM 模式下解密数据
// 密钥与 IV 均为 16 字节；IV 为空时使用 key 前 16 字节作为 IV。
// 参数:
//   - key: 密钥(16 字节)
//   - data: 待解密的密文，可为 []byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密或认证失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(SM4-GCM)
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.Sm4GCMEncrypt(key, "Secret Message", iv)~
// pt = codec.Sm4GCMDecrypt(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(SM4-GCM 解密还原一致)
// assert string(pt) == "Secret Message", "SM4-GCM decrypt should recover plaintext"
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

// SM4EncryptCBCWithPKCSPadding 使用国密 SM4 算法在 CBC 模式下用 PKCS7 填充加密数据
// 密钥与 IV 均为 16 字节；IV 为空时使用 key 前 16 字节作为 IV。
// 注意：Sm4Encrypt、Sm4CBCEncrypt 和 Sm4CBCEncryptWithPKCSPadding 是同一个函数的别名
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: SM4-CBC 加解密
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.Sm4CBCEncrypt(key, "Secret Message", iv)~
// // STDOUT: 解密还原后打印
// println(string(codec.Sm4CBCDecrypt(key, ct, iv)~))   // OUT: Secret Message
// // assert: 锁定结论(SM4-CBC 加解密往返一致)
// assert string(codec.Sm4CBCDecrypt(key, ct, iv)~) == "Secret Message", "SM4-CBC should round-trip"
// ```
func SM4EncryptCBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, CBC)(key, i, iv)
}

// SM4DecryptCBCWithPKCSPadding 使用国密 SM4 算法在 CBC 模式下用 PKCS7 填充解密数据
// 密钥与 IV 均为 16 字节；IV 为空时使用 key 前 16 字节作为 IV。
// 注意：Sm4Decrypt、Sm4CBCDecrypt 和 Sm4CBCDecryptWithPKCSPadding 是同一个函数的别名
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(SM4-CBC)
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.Sm4CBCEncrypt(key, "Secret Message", iv)~
// pt = codec.Sm4CBCDecrypt(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(SM4-CBC 解密还原一致)
// assert string(pt) == "Secret Message", "SM4-CBC decrypt should recover plaintext"
// ```
func SM4DecryptCBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, CBC)(key, i, iv)
}

// SM4EncryptECBWithPKCSPadding 使用国密 SM4 算法在 ECB 模式下用 PKCS7 填充加密数据(ECB 模式下 iv 无用，传 nil)
// 密钥为 16 字节。
// 注意：Sm4ECBEncrypt 和 Sm4ECBEncryptWithPKCSPadding 是同一个函数的别名
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: ECB 模式下无用，传 nil 即可
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: SM4-ECB 加解密(iv 传 nil)
// key = "1234567890123456"
// ct = codec.Sm4ECBEncrypt(key, "Secret Message", nil)~
// // STDOUT: 解密还原后打印
// println(string(codec.Sm4ECBDecrypt(key, ct, nil)~))   // OUT: Secret Message
// // assert: 锁定结论(SM4-ECB 加解密往返一致)
// assert string(codec.Sm4ECBDecrypt(key, ct, nil)~) == "Secret Message", "SM4-ECB should round-trip"
// ```
func SM4EncryptECBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, ECB)(key, i, iv)
}

// SM4DecryptECBWithPKCSPadding 使用国密 SM4 算法在 ECB 模式下用 PKCS7 填充解密数据(ECB 模式下 iv 无用，传 nil)
// 密钥为 16 字节。
// 注意：Sm4ECBDecrypt 和 Sm4ECBDecryptWithPKCSPadding 是同一个函数的别名
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: ECB 模式下无用，传 nil 即可
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(SM4-ECB)
// key = "1234567890123456"
// ct = codec.Sm4ECBEncrypt(key, "Secret Message", nil)~
// pt = codec.Sm4ECBDecrypt(key, ct, nil)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(SM4-ECB 解密还原一致)
// assert string(pt) == "Secret Message", "SM4-ECB decrypt should recover plaintext"
// ```
func SM4DecryptECBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, ECB)(key, i, iv)
}

// SM4EncryptEBCWithPKCSPadding 使用国密 SM4 算法在 ECB 模式下用 PKCS7 填充加密数据(为兼容历史拼写错误保留)
// Deprecated: 请使用 Sm4ECBEncrypt(EBC 是 ECB 的拼写错误)
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: ECB 模式下无用，传 nil 即可
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: 兼容旧拼写的 SM4-ECB 加解密
// key = "1234567890123456"
// ct = codec.Sm4EBCEncrypt(key, "Secret Message", nil)~
// // STDOUT: 解密还原后打印
// println(string(codec.Sm4EBCDecrypt(key, ct, nil)~))   // OUT: Secret Message
// // assert: 锁定结论(加解密往返一致)
// assert string(codec.Sm4EBCDecrypt(key, ct, nil)~) == "Secret Message", "SM4-EBC(alias) should round-trip"
// ```
func SM4EncryptEBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, ECB)(key, i, iv)
}

// SM4DecryptEBCWithPKCSPadding 使用国密 SM4 算法在 ECB 模式下用 PKCS7 填充解密数据(为兼容历史拼写错误保留)
// Deprecated: 请使用 Sm4ECBDecrypt(EBC 是 ECB 的拼写错误)
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: ECB 模式下无用，传 nil 即可
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(兼容旧拼写)
// key = "1234567890123456"
// ct = codec.Sm4EBCEncrypt(key, "Secret Message", nil)~
// pt = codec.Sm4EBCDecrypt(key, ct, nil)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(解密还原一致)
// assert string(pt) == "Secret Message", "SM4-EBC(alias) decrypt should recover plaintext"
// ```
func SM4DecryptEBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, ECB)(key, i, iv)
}

// SM4EncryptCFBWithPKCSPadding 使用国密 SM4 算法在 CFB 模式下用 PKCS7 填充加密数据
// 密钥与 IV 均为 16 字节；IV 为空时使用 key 前 16 字节作为 IV。
// 注意：Sm4CFBEncrypt 和 Sm4CFBEncryptWithPKCSPadding 是同一个函数的别名
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: SM4-CFB 加解密
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.Sm4CFBEncrypt(key, "Secret Message", iv)~
// // STDOUT: 解密还原后打印
// println(string(codec.Sm4CFBDecrypt(key, ct, iv)~))   // OUT: Secret Message
// // assert: 锁定结论(SM4-CFB 加解密往返一致)
// assert string(codec.Sm4CFBDecrypt(key, ct, iv)~) == "Secret Message", "SM4-CFB should round-trip"
// ```
func SM4EncryptCFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, CFB)(key, i, iv)
}

// SM4DecryptCFBWithPKCSPadding 使用国密 SM4 算法在 CFB 模式下用 PKCS7 填充解密数据
// 密钥与 IV 均为 16 字节；IV 为空时使用 key 前 16 字节作为 IV。
// 注意：Sm4CFBDecrypt 和 Sm4CFBDecryptWithPKCSPadding 是同一个函数的别名
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(SM4-CFB)
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.Sm4CFBEncrypt(key, "Secret Message", iv)~
// pt = codec.Sm4CFBDecrypt(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(SM4-CFB 解密还原一致)
// assert string(pt) == "Secret Message", "SM4-CFB decrypt should recover plaintext"
// ```
func SM4DecryptCFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4DecFactory(PKCS5UnPadding, CFB)(key, i, iv)
}

// SM4EncryptOFBWithPKCSPadding 使用国密 SM4 算法在 OFB 模式下用 PKCS7 填充加密数据
// 密钥与 IV 均为 16 字节；IV 为空时使用 key 前 16 字节作为 IV。
// 注意：Sm4OFBEncrypt 和 Sm4OFBEncryptWithPKCSPadding 是同一个函数的别名
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: SM4-OFB 加解密
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.Sm4OFBEncrypt(key, "Secret Message", iv)~
// // STDOUT: 解密还原后打印
// println(string(codec.Sm4OFBDecrypt(key, ct, iv)~))   // OUT: Secret Message
// // assert: 锁定结论(SM4-OFB 加解密往返一致)
// assert string(codec.Sm4OFBDecrypt(key, ct, iv)~) == "Secret Message", "SM4-OFB should round-trip"
// ```
func SM4EncryptOFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return SM4EncFactory(PKCS5Padding, OFB)(key, i, iv)
}

// SM4DecryptOFBWithPKCSPadding 使用国密 SM4 算法在 OFB 模式下用 PKCS7 填充解密数据
// 密钥与 IV 均为 16 字节；IV 为空时使用 key 前 16 字节作为 IV。
// 注意：Sm4OFBDecrypt 和 Sm4OFBDecryptWithPKCSPadding 是同一个函数的别名
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(SM4-OFB)
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.Sm4OFBEncrypt(key, "Secret Message", iv)~
// pt = codec.Sm4OFBDecrypt(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(SM4-OFB 解密还原一致)
// assert string(pt) == "Secret Message", "SM4-OFB decrypt should recover plaintext"
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

// SM4EncFactory 构造国密 SM4 加密函数(用于 CTR 与零填充等模式的 Sm4*Encrypt 系列)
// 由本工厂生成的 SM4 加密函数统一接受 (key, i, iv) 参数，密钥与 IV 均为 16 字节。
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: SM4-CTR 加解密(工厂生成的加密函数之一)
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.Sm4CTREncrypt(key, "Secret Message", iv)~
// // STDOUT: 解密还原后打印
// println(string(codec.Sm4CTRDecrypt(key, ct, iv)~))   // OUT: Secret Message
// // assert: 锁定结论(SM4 工厂加解密往返一致)
// assert string(codec.Sm4CTRDecrypt(key, ct, iv)~) == "Secret Message", "SM4 factory encrypt should round-trip"
// ```
func SM4EncFactory(paddingFunc func([]byte, int) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		data := paddingFunc(interfaceToBytes(i), 16)
		iv = FixIV(iv, key, 16)
		return SM4Enc(key, data, iv, mode)
	}
}

// SM4DecFactory 构造国密 SM4 解密函数(用于 CTR 与零填充等模式的 Sm4*Decrypt 系列)
// 由本工厂生成的 SM4 解密函数统一接受 (key, i, iv) 参数，密钥与 IV 均为 16 字节。
// 参数:
//   - key: 密钥(16 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: SM4-CTR 先加密再解密(工厂生成的解密函数之一)
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.Sm4CTREncrypt(key, "Secret Message", iv)~
// pt = codec.Sm4CTRDecrypt(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(SM4 工厂解密还原一致)
// assert string(pt) == "Secret Message", "SM4 factory decrypt should recover plaintext"
// ```
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

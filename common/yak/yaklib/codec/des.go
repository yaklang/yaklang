package codec

import (
	"crypto/des"
	"fmt"
)

//func desEnc(key []byte, data []byte, iv []byte, mode func(cipher.Block, []byte) cipher.BlockMode, isTripleDES bool) ([]byte, error) {
//	var block cipher.Block
//	var err error
//	if iv == nil {
//		iv = make([]byte, 8)
//	} else {
//		if len(iv)%8 != 0 {
//			iv = ZeroPadding(iv, 8)
//		}
//	}
//	if len(key)%8 != 0 {
//		key = ZeroPadding(key, 8)
//		if len(key) > 8 {
//			key = key[:8]
//		}
//	}
//
//	if isTripleDES {
//		block, err = des.NewTripleDESCipher(key)
//	} else {
//		block, err = des.NewCipher(key)
//	}
//
//	if err != nil {
//		return nil, errors.Errorf("create cipher failed: %s", err)
//	}
//
//	if len(data)%8 != 0 {
//		data = ZeroPadding(data, 8)
//	}
//
//	cbcMode := mode(block, iv)
//	result := make([]byte, len(data))
//	cbcMode.CryptBlocks(result, data)
//	return result, nil
//}
//
//func desDec(key []byte, data []byte, iv []byte, mode func(cipher.Block, []byte) cipher.BlockMode, isTripleDES bool) ([]byte, error) {
//	var block cipher.Block
//	var err error
//	if iv == nil {
//		iv = make([]byte, 8)
//	} else {
//		if len(iv)%8 != 0 {
//			iv = ZeroPadding(iv, 8)
//		}
//	}
//	if len(key)%8 != 0 {
//		key = ZeroPadding(key, 8)
//		if len(key) > 8 {
//			key = key[:8]
//		}
//	}
//
//	if isTripleDES {
//		block, err = des.NewTripleDESCipher(key)
//	} else {
//		block, err = des.NewCipher(key)
//	}
//
//	if err != nil {
//		return nil, errors.Errorf("create cipher failed: %s", err)
//	}
//
//	if len(data)%8 != 0 {
//		data = ZeroPadding(data, 8)
//	}
//
//	cbcMode := mode(block, iv)
//	result := make([]byte, len(data))
//	cbcMode.CryptBlocks(result, data)
//	return result, nil
//}
//
//func DESCBCEncEx(key []byte, data []byte, iv []byte, isTripleDES bool) ([]byte, error) {
//	return desEnc(key, data, iv, cipher.NewCBCEncrypter, isTripleDES)
//}
//
//func DESCBCDecEx(key, data, iv []byte, isTripleDES bool) ([]byte, error) {
//	return desDec(key, data, iv, cipher.NewCBCDecrypter, isTripleDES)
//}
//
//func DESECBEncEx(key []byte, data []byte, isTripleDES bool) ([]byte, error) {
//	var block cipher.Block
//	var err error
//	blockSize := 8
//
//	if len(key)%blockSize != 0 {
//		key = ZeroPadding(key, blockSize)
//		if len(key) > 8 {
//			key = key[:8]
//		}
//	}
//
//	if isTripleDES {
//		block, err = des.NewTripleDESCipher(key)
//	} else {
//		block, err = des.NewCipher(key)
//	}
//	if err != nil {
//		return nil, fmt.Errorf("DES ECB Error: %s", err)
//	}
//	if len(data)%blockSize != 0 {
//		data = ZeroPadding(data, blockSize)
//	}
//
//	encrypted := make([]byte, len(data))
//	for bs, be := 0, blockSize; bs < len(data); bs, be = bs+blockSize, be+blockSize {
//		block.Encrypt(encrypted[bs:be], data[bs:be])
//	}
//	return encrypted, nil
//}
//
//func DESECBDecEx(key []byte, data []byte, isTripleDES bool) ([]byte, error) {
//	var block cipher.Block
//	var err error
//	blockSize := 8
//
//	if len(key)%blockSize != 0 {
//		key = ZeroPadding(key, blockSize)
//		if len(key) > 8 {
//			key = key[:8]
//		}
//	}
//
//	if isTripleDES {
//		block, err = des.NewTripleDESCipher(key)
//	} else {
//		block, err = des.NewCipher(key)
//	}
//	if err != nil {
//		return nil, fmt.Errorf("DES ECB Error: %s", err)
//	}
//	if len(data)%blockSize != 0 {
//		data = ZeroPadding(data, blockSize)
//	}
//
//	decrypted := make([]byte, len(data))
//	for bs, be := 0, blockSize; bs < len(data); bs, be = bs+blockSize, be+blockSize {
//		block.Decrypt(decrypted[bs:be], data[bs:be])
//	}
//	return decrypted, nil
//}
//
//func DESCBCEnc(key []byte, data []byte, iv []byte) ([]byte, error) {
//	return DESCBCEncEx(key, data, iv, false)
//}
//
//func TripleDES_CBCEnc(key []byte, data []byte, iv []byte) ([]byte, error) {
//	return DESCBCEncEx(key, data, iv, true)
//}
//
//func DESCBCDec(key []byte, data []byte, iv []byte) ([]byte, error) {
//	return DESCBCDecEx(key, data, iv, false)
//
//}
//func TripleDES_CBCDec(key []byte, data []byte, iv []byte) ([]byte, error) {
//	return DESCBCDecEx(key, data, iv, true)
//}

// DESECBEnc 使用 DES 算法在 ECB 模式下用零填充加密数据(ECB 模式下无 iv 参数)
// 密钥长度必须是 8 字节。
// 参数:
//   - key: 密钥(8 字节)
//   - data: 待加密的数据字节
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: DES-ECB 加解密(8 字节密钥)
// key = "12345678"
// ct = codec.DESECBEncrypt(key, "Secret Message")~
// // STDOUT: 去零填充解密后打印
// println(string(codec.ZeroUnPadding(codec.DESECBDecrypt(key, ct)~)))   // OUT: Secret Message
// // assert: 锁定结论(DES-ECB 加解密往返一致)
// assert string(codec.ZeroUnPadding(codec.DESECBDecrypt(key, ct)~)) == "Secret Message", "DES-ECB should round-trip"
// ```
func DESECBEnc(key []byte, data []byte) ([]byte, error) {
	return DESEncryptECBWithZeroPadding(key, data, nil)
}

// TripleDES_ECBEnc 使用 3DES(Triple DES) 算法在 ECB 模式下用零填充加密数据(ECB 模式下无 iv 参数)
// 密钥长度必须是 24 字节(即 3 * 8 字节)。
// 参数:
//   - key: 密钥(24 字节)
//   - data: 待加密的数据字节
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: 3DES-ECB 加解密(24 字节密钥)
// key = "123456781234567812345678"
// ct = codec.TripleDESECBEncrypt(key, "Secret Message")~
// // STDOUT: 去零填充解密后打印
// println(string(codec.ZeroUnPadding(codec.TripleDESECBDecrypt(key, ct)~)))   // OUT: Secret Message
// // assert: 锁定结论(3DES-ECB 加解密往返一致)
// assert string(codec.ZeroUnPadding(codec.TripleDESECBDecrypt(key, ct)~)) == "Secret Message", "3DES-ECB should round-trip"
// ```
func TripleDES_ECBEnc(key []byte, data []byte) ([]byte, error) {
	return TripleDESEncryptECBWithZeroPadding(key, data, nil)
}

// DESECBDec 使用 DES 算法在 ECB 模式下用零填充解密数据(ECB 模式下无 iv 参数)
// 密钥长度必须是 8 字节。
// 参数:
//   - key: 密钥(8 字节)
//   - data: 待解密的密文字节
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(DES-ECB)
// key = "12345678"
// ct = codec.DESECBEncrypt(key, "Secret Message")~
// pt = codec.DESECBDecrypt(key, ct)~
// // STDOUT: 去零填充后打印
// println(string(codec.ZeroUnPadding(pt)))   // OUT: Secret Message
// // assert: 锁定结论(DES-ECB 解密还原一致)
// assert string(codec.ZeroUnPadding(pt)) == "Secret Message", "DES-ECB decrypt should recover plaintext"
// ```
func DESECBDec(key []byte, data []byte) ([]byte, error) {
	return DESDecryptECBWithZeroPadding(key, data, nil)
}

// TripleDES_ECBDec 使用 3DES(Triple DES) 算法在 ECB 模式下用零填充解密数据(ECB 模式下无 iv 参数)
// 密钥长度必须是 24 字节(即 3 * 8 字节)。
// 参数:
//   - key: 密钥(24 字节)
//   - data: 待解密的密文字节
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(3DES-ECB)
// key = "123456781234567812345678"
// ct = codec.TripleDESECBEncrypt(key, "Secret Message")~
// pt = codec.TripleDESECBDecrypt(key, ct)~
// // STDOUT: 去零填充后打印
// println(string(codec.ZeroUnPadding(pt)))   // OUT: Secret Message
// // assert: 锁定结论(3DES-ECB 解密还原一致)
// assert string(codec.ZeroUnPadding(pt)) == "Secret Message", "3DES-ECB decrypt should recover plaintext"
// ```
func TripleDES_ECBDec(key []byte, data []byte) ([]byte, error) {
	return TripleDESDecryptECBWithZeroPadding(key, data, nil)
}

// Des
var DESEncryptCBCWithPKCSPadding = DESEncFactory(PKCS5Padding, CBC)

// DESEncryptCBCWithZeroPadding 使用 DES 算法在 CBC 模式下用零填充加密数据
// 密钥长度必须是 8 字节，iv 可为 nil 或 8 字节；iv 为 nil 时固定为密钥或零填充到 8 字节。
// 注意：DESCBCEncrypt、DESEncrypt 和本函数是同一个函数的别名；如需其他填充，先用 codec.PKCS7PaddingForDES 填充再调用。
// 参数:
//   - key: 密钥(8 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(8 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: DES-CBC 加解密(8 字节密钥与 IV)
// key = "12345678"
// iv = "abcdefgh"
// ct = codec.DESCBCEncrypt(key, "Secret Message", iv)~
// // STDOUT: 解密还原后打印
// println(string(codec.DESCBCDecrypt(key, ct, iv)~))   // OUT: Secret Message
// // assert: 锁定结论(DES-CBC 加解密往返一致)
// assert string(codec.DESCBCDecrypt(key, ct, iv)~) == "Secret Message", "DES-CBC should round-trip"
// ```
func DESEncryptCBCWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return DESEncFactory(ZeroPadding, CBC)(key, i, iv)
}

var DESDecryptCBCWithPKCSPadding = DESDecFactory(PKCS5UnPadding, CBC)

// DESDecryptCBCWithZeroPadding 使用 DES 算法在 CBC 模式下用零填充解密数据
// 密钥长度必须是 8 字节，iv 可为 nil 或 8 字节；iv 为 nil 时固定为密钥或零填充到 8 字节。
// 注意：DESCBCDecrypt、DESDecrypt 和本函数是同一个函数的别名
// 参数:
//   - key: 密钥(8 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: 初始化向量(8 字节)，可为 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(DES-CBC)
// key = "12345678"
// iv = "abcdefgh"
// ct = codec.DESCBCEncrypt(key, "Secret Message", iv)~
// pt = codec.DESCBCDecrypt(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(DES-CBC 解密还原一致)
// assert string(pt) == "Secret Message", "DES-CBC decrypt should recover plaintext"
// ```
func DESDecryptCBCWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return DESDecFactory(ZeroUnPadding, CBC)(key, i, iv)
}

var DESEncryptECBWithPKCSPadding = DESEncFactory(PKCS5Padding, ECB)

// DESEncryptECBWithZeroPadding 使用 DES 算法在 ECB 模式下用零填充加密数据(ECB 模式下 iv 无用)
// 密钥长度必须是 8 字节。
// 参数:
//   - key: 密钥(8 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: ECB 模式下无用，传 nil 即可
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: DES-ECB 底层加密(iv 传 nil)
// key = "12345678"
// ct = codec.DESEncryptECBWithZeroPadding(key, "Secret Message", nil)~
// // STDOUT: 去零填充解密后打印
// println(string(codec.ZeroUnPadding(codec.DESDecryptECBWithZeroPadding(key, ct, nil)~)))   // OUT: Secret Message
// // assert: 锁定结论(DES-ECB 零填充往返一致)
// assert string(codec.ZeroUnPadding(codec.DESDecryptECBWithZeroPadding(key, ct, nil)~)) == "Secret Message", "DES-ECB zero-padding should round-trip"
// ```
func DESEncryptECBWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return DESEncFactory(ZeroPadding, ECB)(key, i, iv)
}

var DESDecryptECBWithPKCSPadding = DESDecFactory(PKCS5UnPadding, ECB)
var DESDecryptECBWithZeroPadding = DESDecFactory(ZeroUnPadding, ECB)

// TripleDes
var TripleDESEncryptCBCWithPKCSPadding = TripleDESEncFactory(PKCS5Padding, CBC)

// TripleDESEncryptCBCWithZeroPadding 使用 3DES(Triple DES) 算法在 CBC 模式下用零填充加密数据
// 密钥长度必须是 24 字节(即 3 * 8 字节)，iv 可为 nil 或 8 字节；iv 为 nil 时固定为密钥。
// 注意：TripleDESCBCEncrypt、TripleDESEncrypt 和本函数是同一个函数的别名
// 参数:
//   - key: 密钥(24 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(8 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: 3DES-CBC 加解密(24 字节密钥，8 字节 IV)
// key = "123456781234567812345678"
// iv = "abcdefgh"
// ct = codec.TripleDESCBCEncrypt(key, "Secret Message", iv)~
// // STDOUT: 解密还原后打印
// println(string(codec.TripleDESCBCDecrypt(key, ct, iv)~))   // OUT: Secret Message
// // assert: 锁定结论(3DES-CBC 加解密往返一致)
// assert string(codec.TripleDESCBCDecrypt(key, ct, iv)~) == "Secret Message", "3DES-CBC should round-trip"
// ```
func TripleDESEncryptCBCWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return TripleDESEncFactory(ZeroPadding, CBC)(key, i, iv)
}

var TripleDESDecryptCBCWithPKCSPadding = TripleDESDecFactory(PKCS5UnPadding, CBC)

// TripleDESDecryptCBCWithZeroPadding 使用 3DES(Triple DES) 算法在 CBC 模式下用零填充解密数据
// 密钥长度必须是 24 字节(即 3 * 8 字节)，iv 可为 nil 或 8 字节；iv 为 nil 时固定为密钥或零填充到 8 字节。
// 注意：TripleDESCBCDecrypt、TripleDESDecrypt 和本函数是同一个函数的别名
// 参数:
//   - key: 密钥(24 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: 初始化向量(8 字节)，可为 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(3DES-CBC)
// key = "123456781234567812345678"
// iv = "abcdefgh"
// ct = codec.TripleDESCBCEncrypt(key, "Secret Message", iv)~
// pt = codec.TripleDESCBCDecrypt(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(3DES-CBC 解密还原一致)
// assert string(pt) == "Secret Message", "3DES-CBC decrypt should recover plaintext"
// ```
func TripleDESDecryptCBCWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return TripleDESDecFactory(ZeroUnPadding, CBC)(key, i, iv)
}

var TripleDESEncryptECBWithPKCSPadding = TripleDESEncFactory(PKCS5Padding, ECB)
var TripleDESEncryptECBWithZeroPadding = TripleDESEncFactory(ZeroPadding, ECB)
var TripleDESDecryptECBWithPKCSPadding = TripleDESDecFactory(PKCS5UnPadding, ECB)
var TripleDESDecryptECBWithZeroPadding = TripleDESDecFactory(ZeroUnPadding, ECB)

func DESEncFactory(paddingFunc func([]byte, int) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		data := paddingFunc(interfaceToBytes(i), 8)
		iv = FixIV(iv, key, 8)
		return DESEnc(key, data, iv, mode)
	}
}

func DESDecFactory(unpaddingFunc func([]byte) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		iv = FixIV(iv, key, 8)
		res, err := DESDec(key, interfaceToBytes(i), iv, mode)
		if err != nil {
			return nil, err
		}
		return unpaddingFunc(res), nil
	}
}

func TripleDESEncFactory(paddingFunc func([]byte, int) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		data := paddingFunc(interfaceToBytes(i), 8)
		iv = FixIV(iv, key, 8)
		return TripleDesEnc(key, data, iv, mode)
	}
}

func TripleDESDecFactory(unpaddingFunc func([]byte) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		iv = FixIV(iv, key, 8)
		res, err := TripleDesDec(key, interfaceToBytes(i), iv, mode)
		if err != nil {
			return nil, err
		}
		return unpaddingFunc(res), nil
	}
}

func DESEnc(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	data = ZeroPadding(data, 8)
	c, err := des.NewCipher(key)
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
		return nil, fmt.Errorf("DES: invalid mode %s", mode)
	}
}

func DESDec(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	c, err := des.NewCipher(key)
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
		return nil, fmt.Errorf("DES: invalid mode %s", mode)
	}
}

func TripleDesEnc(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	data = ZeroPadding(data, 8)
	c, err := des.NewTripleDESCipher(key)
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
		return nil, fmt.Errorf("TripleDES: invalid mode %s", mode)
	}
}

func TripleDesDec(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	c, err := des.NewTripleDESCipher(key)
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
		return nil, fmt.Errorf("TripleDES: invalid mode %s", mode)
	}
}

package codec

import (
	"crypto/aes"
	"errors"
	"strconv"
)

//func AESCBCEncryptWithPKCS7Padding(key []byte, i interface{}, iv []byte) ([]byte, error) {
//	return _AESCBCEncryptWithPadding(key, i, iv, PKCS7Padding)
//}
//
//func AESCBCEncryptWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
//	return _AESCBCEncryptWithPadding(key, i, iv, func(i []byte) []byte {
//		return ZeroPadding(i, aes.BlockSize)
//	})
//}
//
//
//func _AESCBCEncryptWithPadding(key []byte, i interface{}, iv []byte, padding func([]byte) []byte) (data []byte, _ error) {
//	origData := interfaceToBytes(i)
//	block, err := aes.NewCipher(key)
//	if err != nil {
//		return nil, err
//	}
//	blockSize := block.BlockSize()
//
//	if iv == nil {
//		iv = key[:blockSize]
//	}
//
//	if len(iv) > blockSize {
//		iv = iv[:blockSize]
//	}
//
//	origData = padding(origData)
//
//	blockMode := cipher.NewCBCEncrypter(block, iv)
//	crypted := make([]byte, len(origData))
//	blockMode.CryptBlocks(crypted, origData)
//	return crypted, nil
//}
//
//func _AESCBCDecryptWithUnpadding(key []byte, i interface{}, iv []byte, unpadding func([]byte) []byte) ([]byte, error) {
//	crypted := interfaceToBytes(i)
//	block, err := aes.NewCipher(key)
//	if err != nil {
//		return nil, err
//	}
//
//	blockSize := block.BlockSize()
//	if iv == nil {
//		iv = key[:blockSize]
//	}
//
//	if len(iv) > blockSize {
//		iv = iv[:blockSize]
//	}
//
//	blockMode := cipher.NewCBCDecrypter(block, iv)
//	origData := make([]byte, len(crypted))
//	blockMode.CryptBlocks(origData, crypted)
//	origData = unpadding(origData)
//	return origData, nil
//}
//
//func AESCBCDecryptWithPKCS7Padding(key []byte, i interface{}, iv []byte) ([]byte, error) {
//	return _AESCBCDecryptWithUnpadding(key, i, iv, PKCS7UnPadding)
//}
//
//func AESCBCDecryptWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
//	return _AESCBCDecryptWithUnpadding(key, i, iv, func(i []byte) []byte {
//		return ZeroUnPadding(i)
//	})
//}

type SymmetricCryptFunc func(key []byte, i interface{}, iv []byte) ([]byte, error)

var AESCBCEncrypt = AESEncryptCBCWithPKCSPadding
var AESCBCDecrypt = AESDecryptCBCWithPKCSPadding

// AESEncryptCBCWithPKCSPadding 使用 AES 算法在 CBC 模式下用 PKCS7 填充加密数据
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)；iv 为 nil 时使用 key 前 16 字节作为 iv。
// 注意：AESCBCEncrypt、AESEncrypt 和 AESCBCEncryptWithPKCS7Padding 是同一个函数的别名
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败(如密钥长度非法)时返回的错误
//
// Example:
// ```
// // VARS: 准备密钥、IV 与明文
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.AESCBCEncrypt(key, "Secret Message", iv)~
// // STDOUT: 解密还原后打印
// println(string(codec.AESCBCDecrypt(key, ct, iv)~))   // OUT: Secret Message
// // assert: 锁定结论(加解密往返一致)
// assert string(codec.AESCBCDecrypt(key, ct, iv)~) == "Secret Message", "AES-CBC encrypt/decrypt should round-trip"
// ```
func AESEncryptCBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(PKCS5Padding, CBC)(key, i, iv)
}

// AESDecryptCBCWithPKCSPadding 使用 AES 算法在 CBC 模式下用 PKCS7 填充解密数据
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)；iv 为 nil 时使用 key 前 16 字节作为 iv。
// 注意：AESCBCDecrypt、AESDecrypt 和 AESCBCDecryptWithPKCS7Padding 是同一个函数的别名
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.AESCBCEncrypt(key, "Secret Message", iv)~
// pt = codec.AESCBCDecrypt(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(解密还原一致)
// assert string(pt) == "Secret Message", "AES-CBC decrypt should recover plaintext"
// ```
func AESDecryptCBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(PKCS5Padding, PKCS5UnPadding, CBC)(key, i, iv)
}

// AESEncryptCBCWithZeroPadding 使用 AES 算法在 CBC 模式下用零(Zero)填充加密数据
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)；iv 为 nil 时使用 key 前 16 字节作为 iv。
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败(如密钥长度非法)时返回的错误
//
// Example:
// ```
// // VARS: 准备密钥、IV 与明文
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.AESCBCEncryptWithZeroPadding(key, "Secret Message", iv)~
// // STDOUT: 解密还原后打印
// println(string(codec.AESCBCDecryptWithZeroPadding(key, ct, iv)~))   // OUT: Secret Message
// // assert: 锁定结论(零填充加解密往返一致)
// assert string(codec.AESCBCDecryptWithZeroPadding(key, ct, iv)~) == "Secret Message", "AES-CBC zero-padding should round-trip"
// ```
func AESEncryptCBCWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(ZeroPadding, CBC)(key, i, iv)
}

// AESDecryptCBCWithZeroPadding 使用 AES 算法在 CBC 模式下用零(Zero)填充解密数据
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)；iv 为 nil 时使用 key 前 16 字节作为 iv。
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 解密还原后的明文字节(末尾零字节会被去除)
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.AESCBCEncryptWithZeroPadding(key, "Secret Message", iv)~
// pt = codec.AESCBCDecryptWithZeroPadding(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(零填充解密还原一致)
// assert string(pt) == "Secret Message", "AES-CBC zero-padding decrypt should recover plaintext"
// ```
func AESDecryptCBCWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(ZeroPadding, ZeroUnPadding, CBC)(key, i, iv)
}

func AESEncFactory(paddingFunc func([]byte, int) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		// 验证密钥长度必须是 16、24 或 32 字节
		keyLen := len(key)
		if keyLen != 16 && keyLen != 24 && keyLen != 32 {
			return nil, errors.New("AES key length must be 16, 24, or 32 bytes, got " + strconv.Itoa(keyLen) + " bytes")
		}
		data := paddingFunc(interfaceToBytes(i), 16)
		iv = FixIV(iv, key, 16)
		return AESEnc(key, data, iv, mode)
	}
}

func AESDecFactory(paddingFunc func([]byte, int) []byte, unpaddingFunc func([]byte) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		// 验证密钥长度必须是 16、24 或 32 字节
		keyLen := len(key)
		if keyLen != 16 && keyLen != 24 && keyLen != 32 {
			return nil, errors.New("AES key length must be 16, 24, or 32 bytes, got " + strconv.Itoa(keyLen) + " bytes")
		}
		iv = FixIV(iv, key, 16)
		data := interfaceToBytes(i)
		// Auto-pad data to blockSize (16 bytes) multiple if needed
		// This allows decryption of data that is not a multiple of block size
		// Only pad if data is not already a multiple of block size
		blockSize := 16
		if len(data)%blockSize != 0 {
			data = paddingFunc(data, blockSize)
		}

		res, err := AESDec(key, data, iv, mode)
		if err != nil {
			return nil, err
		}
		return unpaddingFunc(res), nil
	}
}

// isAESStreamMode 判断 AES 模式是否为流模式
// 流模式（CTR、CFB、OFB）不需要 padding，明文长度等于密文长度
// 块模式（CBC、ECB）需要 padding，要求数据长度对齐到块大小
func isAESStreamMode(mode string) bool {
	return mode == CTR || mode == CFB || mode == OFB
}

// AESEncryptBasic 使用 AES 算法对数据进行加密，支持多种模式(CBC、CFB、ECB、OFB、CTR)
// 注意：此函数是底层高级用法，需要外部自行处理 padding、key、iv 等问题。
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - data: 待加密的数据字节
//   - iv: 初始化向量(块模式需要)
//   - mode: 加密模式，取 codec.CBC / codec.CFB / codec.ECB / codec.OFB / codec.CTR
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: 底层加密，块模式内部会做零填充
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.AESEncryptBasic(key, "Secret Message", iv, codec.CBC)~
// dec = codec.AESDecryptBasic(key, ct, iv, codec.CBC)~
// // STDOUT: 去零填充后打印
// println(string(codec.ZeroUnPadding(dec)))   // OUT: Secret Message
// // assert: 锁定结论(底层加解密往返一致)
// assert string(codec.ZeroUnPadding(dec)) == "Secret Message", "AESEncryptBasic/AESDecryptBasic should round-trip"
// ```
func AESEnc(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	// 只对块模式（CBC、ECB）进行 padding，流模式（CTR、CFB、OFB）不需要 padding
	if !isAESStreamMode(mode) {
		data = ZeroPadding(data, aes.BlockSize)
	}

	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	switch mode {
	case CBC:
		return CBCEncode(c, iv, data)
	case CFB:
		return CFBEncode(c, iv, data)
	case ECB:
		return ECBEncode(c, data)
	case OFB:
		return OFBEncode(c, iv, data)
	case CTR:
		return CTREncode(c, iv, data)
	default:
		return nil, errors.New("AES: invalid mode")
	}
}

// AESDecryptBasic 使用 AES 算法对数据进行解密，支持多种模式(CBC、CFB、ECB、OFB、CTR)
// 注意：此函数是底层高级用法，需要外部自行处理 padding、key、iv 等问题。
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - data: 待解密的密文字节
//   - iv: 初始化向量(块模式需要)
//   - mode: 解密模式，取 codec.CBC / codec.CFB / codec.ECB / codec.OFB / codec.CTR
//
// 返回值:
//   - []byte: 解密后的明文字节(块模式下可能含零填充)
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 底层加解密
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.AESEncryptBasic(key, "Secret Message", iv, codec.CBC)~
// dec = codec.AESDecryptBasic(key, ct, iv, codec.CBC)~
// // STDOUT: 去零填充后打印
// println(string(codec.ZeroUnPadding(dec)))   // OUT: Secret Message
// // assert: 锁定结论(底层解密还原一致)
// assert string(codec.ZeroUnPadding(dec)) == "Secret Message", "AESDecryptBasic should recover plaintext"
// ```
func AESDec(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	switch mode {
	case CBC:
		return CBCDecode(c, iv, data)
	case CFB:
		return CFBDecode(c, iv, data)
	case ECB:
		return ECBDecode(c, data)
	case OFB:
		return OFBDecode(c, iv, data)
	case CTR:
		return CTRDecode(c, iv, data)
	default:
		return nil, errors.New("AES: invalid mode")
	}
}

func AESEncWithPassphrase(passphrase, data, salt []byte, KDF KeyDerivationFunc, aesMode string) ([]byte, error) {
	key, iv, err := KDF(passphrase, salt)
	if err != nil {
		return nil, errors.New("OpensslAESEnc: generate key failed: " + err.Error())
	}
	return AESEnc(key, data, iv, aesMode)
}

func AESDecWithPassphrase(passphrase, data, salt []byte, KDF KeyDerivationFunc, aesMode string) ([]byte, error) {
	key, iv, err := KDF(passphrase, salt)
	if err != nil {
		return nil, errors.New("OpensslAESDnc: generate key failed: " + err.Error())
	}
	return AESDec(key, data, iv, aesMode)
}

package codec

//func _AESECBEncryptWithPadding(key []byte, i interface{}, iv []byte, padding func(i []byte) []byte) ([]byte, error) {
//	data := interfaceToBytes(i)
//	block, err := aes.NewCipher(key)
//	if err != nil {
//		return nil, err
//	}
//
//	data = padding(data)
//
//	encrypted := make([]byte, len(data))
//	size := block.BlockSize()
//	if iv == nil {
//		iv = key[:size]
//	}
//
//	for bs, be := 0, size; bs < len(data); bs, be = bs+size, be+size {
//		block.Encrypt(encrypted[bs:be], data[bs:be])
//	}
//	return encrypted, nil
//}
//
//func AESECBEncrypt(key []byte, i interface{}, iv []byte) ([]byte, error) {
//	return _AESECBEncryptWithPadding(key, i, iv, PKCS7Padding)
//}
//
//var AESECBEncryptWithPKCS7Padding = AESECBEncrypt
//
//func AESECBEncryptWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
//	return _AESECBEncryptWithPadding(key, i, iv, func(i []byte) []byte {
//		return ZeroPadding(i, aes.BlockSize)
//	})
//}
//
//func _AESECBDecryptWithPadding(key []byte, i interface{}, iv []byte, padding func([]byte) []byte) ([]byte, error) {
//	crypted := interfaceToBytes(i)
//	block, err := aes.NewCipher(key)
//	if err != nil {
//		return nil, err
//	}
//
//	decrypted := make([]byte, len(crypted))
//	size := block.BlockSize()
//	if iv == nil {
//		iv = key[:size]
//	}
//	if len(iv) < size {
//		iv = padding(iv)
//	} else if len(iv) > size {
//		iv = iv[:size]
//	}
//
//	//if len(crypted)%block.BlockSize() != 0 {
//	//	panic("crypto/cipher: input not full blocks")
//	//}
//	//if len(decrypted) < len(crypted) {
//	//	panic("crypto/cipher: output smaller than input")
//	//}
//
//	for bs, be := 0, size; bs < len(crypted); bs, be = bs+size, be+size {
//		block.Decrypt(decrypted[bs:be], crypted[bs:be])
//	}
//
//	decrypted = padding(decrypted)
//	return decrypted, nil
//}
//
//func AESECBDecryptWithPKCS7Padding(key []byte, i interface{}, iv []byte) ([]byte, error) {
//	return _AESECBDecryptWithPadding(key, i, iv, PKCS7UnPadding)
//}
//func AESECBDecryptWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
//	return _AESECBDecryptWithPadding(key, i, iv, func(i []byte) []byte {
//		return ZeroUnPadding(i)
//	})
//}

var AESECBDecrypt = AESDecryptECBWithPKCSPadding
var AESECBEncrypt = AESEncryptECBWithPKCSPadding

// AESEncryptECBWithPKCSPadding 使用 AES 算法在 ECB 模式下用 PKCS7 填充加密数据(ECB 模式下 iv 无用，传 nil)
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)。
// 注意：AESECBEncrypt 和 AESECBEncryptWithPKCS7Padding 是同一个函数的别名
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: ECB 模式下无用，传 nil 即可
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败(如密钥长度非法)时返回的错误
//
// Example:
// ```
// // VARS: ECB 模式 iv 传 nil
// key = "1234567890123456"
// ct = codec.AESECBEncrypt(key, "Secret Message", nil)~
// // STDOUT: 解密还原后打印
// println(string(codec.AESECBDecrypt(key, ct, nil)~))   // OUT: Secret Message
// // assert: 锁定结论(ECB 加解密往返一致)
// assert string(codec.AESECBDecrypt(key, ct, nil)~) == "Secret Message", "AES-ECB encrypt/decrypt should round-trip"
// ```
func AESEncryptECBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(PKCS5Padding, ECB)(key, i, iv)
}

// AESEncryptECBWithZeroPadding 使用 AES 算法在 ECB 模式下用零(Zero)填充加密数据(ECB 模式下 iv 无用，传 nil)
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)。
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: ECB 模式下无用，传 nil 即可
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败(如密钥长度非法)时返回的错误
//
// Example:
// ```
// // VARS: ECB 零填充，iv 传 nil
// key = "1234567890123456"
// ct = codec.AESECBEncryptWithZeroPadding(key, "Secret Message", nil)~
// // STDOUT: 解密还原后打印
// println(string(codec.AESECBDecryptWithZeroPadding(key, ct, nil)~))   // OUT: Secret Message
// // assert: 锁定结论(ECB 零填充往返一致)
// assert string(codec.AESECBDecryptWithZeroPadding(key, ct, nil)~) == "Secret Message", "AES-ECB zero-padding should round-trip"
// ```
func AESEncryptECBWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(ZeroPadding, ECB)(key, i, iv)
}

// AESDecryptECBWithPKCSPadding 使用 AES 算法在 ECB 模式下用 PKCS7 填充解密数据(ECB 模式下 iv 无用，传 nil)
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)。
// 注意：AESECBDecrypt 和 AESDecryptECBWithPKCSPadding 是同一个函数的别名
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: ECB 模式下无用，传 nil 即可
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(ECB iv 传 nil)
// key = "1234567890123456"
// ct = codec.AESECBEncrypt(key, "Secret Message", nil)~
// pt = codec.AESECBDecrypt(key, ct, nil)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(ECB 解密还原一致)
// assert string(pt) == "Secret Message", "AES-ECB decrypt should recover plaintext"
// ```
func AESDecryptECBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(PKCS5Padding, PKCS5UnPadding, ECB)(key, i, iv)
}

// AESDecryptECBWithZeroPadding 使用 AES 算法在 ECB 模式下用零(Zero)填充解密数据(ECB 模式下 iv 无用，传 nil)
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)。
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - i: 待解密的密文，可为 []byte 等
//   - iv: ECB 模式下无用，传 nil 即可
//
// 返回值:
//   - []byte: 解密还原后的明文字节(末尾零字节会被去除)
//   - error: 解密失败时返回的错误
//
// Example:
// ```
// // VARS: 先加密再解密(ECB 零填充)
// key = "1234567890123456"
// ct = codec.AESECBEncryptWithZeroPadding(key, "Secret Message", nil)~
// pt = codec.AESECBDecryptWithZeroPadding(key, ct, nil)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(ECB 零填充解密还原一致)
// assert string(pt) == "Secret Message", "AES-ECB zero-padding decrypt should recover plaintext"
// ```
func AESDecryptECBWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(ZeroPadding, ZeroUnPadding, ECB)(key, i, iv)
}

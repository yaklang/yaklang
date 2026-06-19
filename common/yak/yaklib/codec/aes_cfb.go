package codec

// AESEncryptCFBWithPKCSPadding 使用 AES 算法在 CFB 模式下用 PKCS7 填充加密数据
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)；iv 为 nil 时使用 key 前 16 字节作为 iv。
// 注意：AESCFBEncrypt 和 AESEncryptCFBWithPKCSPadding 是同一个函数的别名
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: CFB 流密码模式
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.AESCFBEncrypt(key, "Secret Message", iv)~
// // STDOUT: 解密还原后打印
// println(string(codec.AESCFBDecrypt(key, ct, iv)~))   // OUT: Secret Message
// // assert: 锁定结论(CFB 加解密往返一致)
// assert string(codec.AESCFBDecrypt(key, ct, iv)~) == "Secret Message", "AES-CFB encrypt/decrypt should round-trip"
// ```
func AESEncryptCFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(PKCS5Padding, CFB)(key, i, iv)
}

// AESDecryptCFBWithPKCSPadding 使用 AES 算法在 CFB 模式下用 PKCS7 填充解密数据
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)；iv 为 nil 时使用 key 前 16 字节作为 iv。
// 注意：AESCFBDecrypt 和 AESDecryptCFBWithPKCSPadding 是同一个函数的别名
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
// // VARS: 先加密再解密(CFB)
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.AESCFBEncrypt(key, "Secret Message", iv)~
// pt = codec.AESCFBDecrypt(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(CFB 解密还原一致)
// assert string(pt) == "Secret Message", "AES-CFB decrypt should recover plaintext"
// ```
func AESDecryptCFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(PKCS5Padding, PKCS5UnPadding, CFB)(key, i, iv)
}

// AESEncryptCFBWithZeroPadding 使用 AES 算法在 CFB 模式下用零(Zero)填充加密数据
// 密钥长度必须是 16/24/32 字节(分别对应 AES-128/192/256)；iv 为 nil 时使用 key 前 16 字节作为 iv。
// 参数:
//   - key: 密钥(16/24/32 字节)
//   - i: 待加密的数据，可为 string、[]byte 等
//   - iv: 初始化向量(16 字节)，可为 nil
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 加密失败时返回的错误
//
// Example:
// ```
// // VARS: CFB 零填充加解密
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.AESEncryptCFBWithZeroPadding(key, "Secret Message", iv)~
// // STDOUT: 去零填充解密后打印
// println(string(codec.ZeroUnPadding(codec.AESDecryptCFBWithZeroPadding(key, ct, iv)~)))   // OUT: Secret Message
// // assert: 锁定结论(CFB 零填充往返一致)
// assert string(codec.ZeroUnPadding(codec.AESDecryptCFBWithZeroPadding(key, ct, iv)~)) == "Secret Message", "AES-CFB zero-padding should round-trip"
// ```
func AESEncryptCFBWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(ZeroPadding, CFB)(key, i, iv)
}

// AESDecryptCFBWithZeroPadding 使用 AES 算法在 CFB 模式下用零(Zero)填充解密数据
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
// // VARS: 先加密再解密(CFB 零填充)
// key = "1234567890123456"
// iv = "abcdefghijklmnop"
// ct = codec.AESEncryptCFBWithZeroPadding(key, "Secret Message", iv)~
// pt = codec.AESDecryptCFBWithZeroPadding(key, ct, iv)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(CFB 零填充解密还原一致)
// assert string(pt) == "Secret Message", "AES-CFB zero-padding decrypt should recover plaintext"
// ```
func AESDecryptCFBWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(ZeroPadding, ZeroUnPadding, CFB)(key, i, iv)
}

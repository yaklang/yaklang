package codec

// AESEncryptCFBWithPKCSPadding 使用 AES 算法，在 CFB 模式下，使用 PKCS5 填充来加密数据。
// 密钥的长度必须是 16、24 或 32 字节（分别对应 AES-128、AES-192 或 AES-256）。
// 如果iv为 nil，则使用key的前16字节作为iv。
func AESEncryptCFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(PKCS5Padding, CFB)(key, i, iv)
}

// AESCFBDecryptWithPKCS7Padding 使用 AES 算法，在 CFB 模式下，使用 PKCS5 填充来解密数据。
// 密钥的长度必须是 16、24 或 32 字节（分别对应 AES-128、AES-192 或 AES-256）。
// 如果iv为 nil，则使用key的前16字节作为iv。
func AESDecryptCFBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(PKCS5Padding, PKCS5UnPadding, CFB)(key, i, iv)
}

// AESCFBEncryptWithZeroPadding 使用 AES 算法，在 CFB 模式下，使用 Zero 填充来加密数据。
func AESEncryptCFBWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(ZeroPadding, CFB)(key, i, iv)
}

// AESCFBDecryptWithZeroPadding 使用 AES 算法，在 CFB 模式下，使用 Zero 填充来解密数据。
func AESDecryptCFBWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(ZeroPadding, ZeroUnPadding, CFB)(key, i, iv)
}

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

// AESCBCEncryptWithZeroPadding 使用 AES 算法，在 ECB 模式下对数据进行加密，使用 PKCSPadding 填充方式
// 它接受一个密钥（key）、需要加密的数据（data to encrypt）。
// ecb 模式下iv 无用。
// 密钥的长度必须是 16、24 或 32 字节（分别对应 AES-128、AES-192 或 AES-256）。
// AESECBEncrypt 和 AESECBEncryptWithPKCSPadding 是同一个函数。
// example:
// ```
// codec.AESECBEncryptWithPKCS7Padding("1234567890123456", "hello world", nil)
// ```
func AESEncryptECBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(PKCS5Padding, ECB)(key, i, iv)
}

// AESCBCEncryptWithZeroPadding 使用 AES 算法，在 ECB 模式下对数据进行加密，使用 ZeroPadding 填充方式
// 它接受一个密钥（key）、需要加密的数据（data to encrypt）。
// ecb 模式下iv 无用。
// 密钥的长度必须是 16、24 或 32 字节（分别对应 AES-128、AES-192 或 AES-256）。
// example:
// ```
// codec.AESECBEncryptWithZeroPadding("1234567890123456", "hello world", nil)
// ```
func AESEncryptECBWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(ZeroPadding, ECB)(key, i, iv)
}

// AESDecryptECBWithPKCSPadding 使用 AES 算法，在 ECB 模式下对数据进行解密，使用 PKCSPadding 填充方式
// 它接受一个密钥（key）、需要解密的数据（data to decrypt）。
// ecb 模式下iv 无用。
// 密钥的长度必须是 16、24 或 32 字节（分别对应 AES-128、AES-192 或 AES-256）。
// AESECBDecrypt 和 AESDecryptECBWithPKCSPadding 是同一个函数。
// example:
// ```
// codec.AESECBDecryptWithPKCS7Padding("1234567890123456", "hello world", nil)
// ```
func AESDecryptECBWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(PKCS5Padding, PKCS5UnPadding, ECB)(key, i, iv)
}

// AESDecryptECBWithZeroPadding 使用 AES 算法，在 ECB 模式下对数据进行解密，使用 ZeroPadding 填充方式
// 它接受一个密钥（key）、需要解密的数据（data to decrypt）。
// ecb 模式下iv 无用。
// 密钥的长度必须是 16、24 或 32 字节（分别对应 AES-128、AES-192 或 AES-256）。
// example:
// ```
// codec.AESECBDecryptWithZeroPadding("1234567890123456", "hello world", nil)
// ```
func AESDecryptECBWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(ZeroPadding, ZeroUnPadding, ECB)(key, i, iv)
}

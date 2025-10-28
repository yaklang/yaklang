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

// DESECBEncrypt 是一个便捷函数，用于使用 DES 算法，在 ECB 模式下，使用 零填充来加密数据。
// 它接受一个密钥（key）、需要加密的数据（data to encrypt）（ecb 模式下 iv 无用）
// 密钥的长度必须是 8 个字节。
// example:
// ```
// codec.DESECBEncrypt([]byte("12345678"), "hello world")
// ```
func DESECBEnc(key []byte, data []byte) ([]byte, error) {
	return DESEncryptECBWithZeroPadding(key, data, nil)
}

// TripleDESECBEncrypt 是一个便捷函数，用于使用 Triple DES 算法，在 ECB 模式下，使用 零填充来加密数据。
// 它接受一个密钥（key）、需要加密的数据（data to encrypt）（ecb 模式下 iv 无用）
// 密钥的长度必须是 24 个字节（即 3 * 8 字节）。
// example:
// ```
// codec.TripleDESECBEncrypt([]byte("123456789012345678901234"), "hello world")
// ```
func TripleDES_ECBEnc(key []byte, data []byte) ([]byte, error) {
	return TripleDESEncryptECBWithZeroPadding(key, data, nil)
}

// DESECBDecrypt 是一个便捷函数，用于使用 DES 算法，在 ECB 模式下，使用 零填充来解密数据。
// 它接受一个密钥（key）、需要解密的数据（data to decrypt）（ecb 模式下 iv 无用）
// 密钥的长度必须是 8 个字节。
// example:
// ```
// codec.DESECBDecrypt([]byte("12345678"), ciphertext)
// ```
func DESECBDec(key []byte, data []byte) ([]byte, error) {
	return DESDecryptECBWithZeroPadding(key, data, nil)
}

// TripleDESECBDecrypt 是一个便捷函数，用于使用 Triple DES 算法，在 ECB 模式下，使用 零填充来解密数据。
// 它接受一个密钥（key）、需要解密的数据（data to decrypt）（ecb 模式下 iv 无用）
// 密钥的长度必须是 24 个字节（即 3 * 8 字节）。
// example:
// ```
// codec.TripleDESECBDecrypt([]byte("123456789012345678901234"), ciphertext)
// ```
func TripleDES_ECBDec(key []byte, data []byte) ([]byte, error) {
	return TripleDESDecryptECBWithZeroPadding(key, data, nil)
}

// Des
var DESEncryptCBCWithPKCSPadding = DESEncFactory(PKCS5Padding, CBC)

// DESCBCEncrypt 是一个便捷函数，用于使用 DES 算法，在 CBC 模式下，使用零填充来加密数据。
// 它接受一个密钥（key）、需要加密的数据（data to encrypt）和一个初始化向量（iv）。
// 密钥的长度必须是 8 个字节，并且 iv 可以是 nil 或者 8 个字节长。
// 如果 iv 为 nil，它将被固定为密钥，或者用零填充到 8 个字节。
// 加密数据长度需要是8的倍数。默认使用零填充方式进行填充。
// 如果希望使用其他填充方式，请使用 codec.PKCS7PaddingForDES 进行填充后，再调用此函数进行加密。
// DESCBCEncrypt DESEncrypt 是同一个函数。
// example:
// ```
// codec.DESCBCEncrypt([]byte("12345678"), "hello world", "12345678")
// ```
func DESEncryptCBCWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return DESEncFactory(ZeroPadding, CBC)(key, i, iv)
}

var DESDecryptCBCWithPKCSPadding = DESDecFactory(PKCS5UnPadding, CBC)

// DESCBCDecrypt 是一个便捷函数，用于使用 DES 算法，在 CBC 模式下，使用零填充来解密数据。
// 它接受一个密钥（key）、需要解密的数据（data to decrypt）和一个初始化向量（iv）。
// 密钥的长度必须是 8 个字节，并且 iv 可以是 nil 或者 8 个字节长。
// 如果 iv 为 nil，它将被固定为密钥，或者用零填充到 8 个字节。
// DESCBCDecrypt DESDecrypt 是同一个函数。
// example:
// ```
// codec.DESCBCEncrypt([]byte("12345678"), ciphertext, "12345678")
// ```
func DESDecryptCBCWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return DESDecFactory(ZeroUnPadding, CBC)(key, i, iv)
}

var DESEncryptECBWithPKCSPadding = DESEncFactory(PKCS5Padding, ECB)

// DESECBEncrypt 是一个便捷函数，用于使用 DES 算法，在 ECB 模式下，使用 零填充来加密数据。
// 它接受一个密钥（key）、需要加密的数据（data to encrypt）。
// ecb 模式下 iv 无用。
// 密钥的长度必须是 8 个字节。
// example:
// ```
// codec.DESECBEncrypt([]byte("12345678"), "hello world", nil)
// ```
func DESEncryptECBWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return DESEncFactory(ZeroPadding, ECB)(key, i, iv)
}

var DESDecryptECBWithPKCSPadding = DESDecFactory(PKCS5UnPadding, ECB)
var DESDecryptECBWithZeroPadding = DESDecFactory(ZeroUnPadding, ECB)

// TripleDes
var TripleDESEncryptCBCWithPKCSPadding = TripleDESEncFactory(PKCS5Padding, CBC)

// TripleDESCBCEncrypt 是一个便捷函数，用于使用 Triple DES 算法，在 CBC 模式下，使用 零填充来加密数据。
// 它接受一个密钥（key）、需要加密的数据（data to encrypt）和一个初始化向量（iv）。
// 密钥的长度必须是 24 个字节（即 3 * 8 字节），并且 iv 可以是 nil 或者 8 个字节长。
// 如果 iv 为 nil，它将被固定为密钥.
// TripleDESCBCDecrypt TripleDESEncrypt 是同一个函数。
// example:
// ```
// codec.TripleDESCBCEncrypt([]byte("123456789012345678901234"), "hello world", "12345678")
// ```
func TripleDESEncryptCBCWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return TripleDESEncFactory(ZeroPadding, CBC)(key, i, iv)
}

var TripleDESDecryptCBCWithPKCSPadding = TripleDESDecFactory(PKCS5UnPadding, CBC)

// TripleDESCBCDecrypt 是一个便捷函数，用于使用 Triple DES 算法，在 CBC 模式下，使用 零填充来解密数据。
// 它接受一个密钥（key）、需要解密的数据（data to decrypt）和一个初始化向量（iv）。
// 密钥的长度必须是 24 个字节（即 3 * 8 字节），并且 iv 可以是 nil 或者 8 个字节长。
// 如果 iv 为 nil，它将被固定为密钥，或者用零填充到 8 个字节。
// TripleDESCBCDecrypt TripleDESDecrypt 是同一个函数。
// example:
// ```
// codec.TripleDESCBCDecrypt([]byte("123456789012345678901234"), ciphertext, "12345678")
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

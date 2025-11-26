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

// AESCBCEncryptWithPKCS7Padding 使用 AES 算法，在 CBC 模式下，使用 PKCS5 填充来加密数据。
// 它接受一个密钥（key）、需要加密的数据（data to encrypt）和一个初始化向量（iv）。
// 密钥的长度必须是 16、24 或 32 字节（分别对应 AES-128、AES-192 或 AES-256）。
// 如果iv为 nil，则使用key的前16字节作为iv。
// 注意：AESCBCEncrypt AESEncrypt 和 AESCBCEncryptWithPKCS7Padding 是同一个函数的别名
// example：
// ```
//
//	codec.AESCBCEncryptWithPKCS7Padding("1234567890123456", "hello world", "1234567890123456")
//
// ```
func AESEncryptCBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(PKCS5Padding, CBC)(key, i, iv)
}

// AESCBCDecryptWithPKCS7Padding 使用 AES 算法，在 CBC 模式下，使用 PKCS5 填充来解密数据。
// 它接受一个密钥（key）、需要解密的数据（data to decrypt）和一个初始化向量（iv）。
// 密钥的长度必须是 16、24 或 32 字节（分别对应 AES-128、AES-192 或 AES-256）。
// 如果iv为 nil，则使用key的前16字节作为iv。
// 注意：AESCBCDecrypt AESDecrypt 和 AESCBCDecryptWithPKCS7Padding 是同一个函数的别名
// example：
// ```
//
//	codec.AESCBCDecryptWithPKCS7Padding("1234567890123456", ciphertext, "1234567890123456")
//
// ```
func AESDecryptCBCWithPKCSPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESDecFactory(PKCS5Padding, PKCS5UnPadding, CBC)(key, i, iv)
}

// AESCBCEncryptWithZeroPadding 使用 AES 算法，在 CBC 模式下，使用 Zero 填充来加密数据。
// 它接受一个密钥（key）、需要加密的数据（data to encrypt）和一个初始化向量（iv）。
// 密钥的长度必须是 16、24 或 32 字节（分别对应 AES-128、AES-192 或 AES-256）。
// 如果iv为 nil，则使用key的前16字节作为iv。
// example：
// ```
// codec.AESCBCEncryptWithZeroPadding("1234567890123456", "hello world", "1234567890123456")
// ```
func AESEncryptCBCWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return AESEncFactory(ZeroPadding, CBC)(key, i, iv)
}

// AESCBCDecryptWithZeroPadding 使用 AES 算法，在 CBC 模式下，使用 Zero 填充来解密数据。
// 它接受一个密钥（key）、需要解密的数据（data to decrypt）和一个初始化向量（iv）。
// 密钥的长度必须是 16、24 或 32 字节（分别对应 AES-128、AES-192 或 AES-256）。
// 如果iv为 nil，则使用key的前16字节作为iv。
// example：
// ```
// codec.AESCBCDecryptWithZeroPadding("1234567890123456", ciphertext, "1234567890123456")
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

// AESEncryptBasic 使用 AES 算法对数据进行加密，支持多种模式（CBC、CFB、ECB、OFB、CTR） 。
// 注意：此函数是底层的高级用法，需要外部自行处理 padding，key，iv 等问题。
// example：
// ```
//
//	codec.AESEncryptBasic("1234567890123456", codec.PKCS5Padding("hello world",16), "1234567890123456", codec.CBC)
//
// ```
func AESEnc(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	data = ZeroPadding(data, aes.BlockSize) // 交给外部处理 padding问题，内部自动 zero padding避免外部传入padding后的数据后多次padding的同时，保证数据块正常
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

// AESDecryptBasic 使用 AES 算法对数据进行解密，支持多种模式（CBC、CFB、ECB、OFB、CTR）。
// 注意：此函数是底层的高级用法，需要外部自行处理 padding，key，iv 等问题。
// example：
// ```
//
//	codec.AESDecryptBasic("1234567890123456", cipertext, "1234567890123456", codec.CBC)
//
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

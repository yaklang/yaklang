package codec

import (
	"crypto/aes"
	"errors"
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

var AESEncryptCBCWithPKCSPadding = AESEncFactory(PKCS5Padding, CBC)
var AESEncryptCBCWithZeroPadding = AESEncFactory(ZeroPadding, CBC)
var AESDecryptCBCWithPKCSPadding = AESDecFactory(PKCS5UnPadding, CBC)
var AESDecryptCBCWithZeroPadding = AESDecFactory(ZeroUnPadding, CBC)

func AESEncFactory(paddingFunc func([]byte, int) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		data := paddingFunc(interfaceToBytes(i), 16)
		iv = FixIV(iv, key, 16)
		return AESEnc(key, data, iv, mode)
	}
}

func AESDecFactory(unpaddingFunc func([]byte) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		iv = FixIV(iv, key, 16)
		res, err := AESDec(key, interfaceToBytes(i), iv, mode)
		if err != nil {
			return nil, err
		}
		return unpaddingFunc(res), nil
	}
}

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

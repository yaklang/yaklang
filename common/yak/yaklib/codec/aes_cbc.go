package codec

import (
	"crypto/aes"
	"crypto/cipher"
)

func AESCBCEncryptWithPKCS7Padding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return _AESCBCEncryptWithPadding(key, i, iv, PKCS7Padding)
}

func AESCBCEncryptWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return _AESCBCEncryptWithPadding(key, i, iv, func(i []byte) []byte {
		return ZeroPadding(i, aes.BlockSize)
	})
}

var AESCBCEncrypt = AESCBCEncryptWithPKCS7Padding

func _AESCBCEncryptWithPadding(key []byte, i interface{}, iv []byte, padding func([]byte) []byte) (data []byte, _ error) {
	origData := interfaceToBytes(i)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()

	if iv == nil {
		iv = key[:blockSize]
	}

	if len(iv) > blockSize {
		iv = iv[:blockSize]
	}

	origData = padding(origData)

	blockMode := cipher.NewCBCEncrypter(block, iv)
	crypted := make([]byte, len(origData))
	blockMode.CryptBlocks(crypted, origData)
	return crypted, nil
}

func _AESCBCDecryptWithUnpadding(key []byte, i interface{}, iv []byte, unpadding func([]byte) []byte) ([]byte, error) {
	crypted := interfaceToBytes(i)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize()
	if iv == nil {
		iv = key[:blockSize]
	}

	if len(iv) > blockSize {
		iv = iv[:blockSize]
	}

	blockMode := cipher.NewCBCDecrypter(block, iv)
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)
	origData = unpadding(origData)
	return origData, nil
}

func AESCBCDecryptWithPKCS7Padding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return _AESCBCDecryptWithUnpadding(key, i, iv, PKCS7UnPadding)
}

func AESCBCDecryptWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return _AESCBCDecryptWithUnpadding(key, i, iv, func(i []byte) []byte {
		return ZeroUnPadding(i)
	})
}

var AESCBCDecrypt = AESCBCDecryptWithPKCS7Padding

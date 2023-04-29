package codec

import "crypto/aes"

func _AESECBEncryptWithPadding(key []byte, i interface{}, iv []byte, padding func(i []byte) []byte) ([]byte, error) {
	data := interfaceToBytes(i)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	data = padding(data)

	encrypted := make([]byte, len(data))
	size := block.BlockSize()
	if iv == nil {
		iv = key[:size]
	}

	for bs, be := 0, size; bs < len(data); bs, be = bs+size, be+size {
		block.Encrypt(encrypted[bs:be], data[bs:be])
	}
	return encrypted, nil
}

func AESECBEncrypt(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return _AESECBEncryptWithPadding(key, i, iv, PKCS7Padding)
}

var AESECBEncryptWithPKCS7Padding = AESECBEncrypt

func AESECBEncryptWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return _AESECBEncryptWithPadding(key, i, iv, func(i []byte) []byte {
		return ZeroPadding(i, aes.BlockSize)
	})
}

func _AESECBDecryptWithPadding(key []byte, i interface{}, iv []byte, padding func([]byte) []byte) ([]byte, error) {
	crypted := interfaceToBytes(i)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	decrypted := make([]byte, len(crypted))
	size := block.BlockSize()
	if iv == nil {
		iv = key[:size]
	}
	if len(iv) < size {
		iv = padding(iv)
	} else if len(iv) > size {
		iv = iv[:size]
	}

	for bs, be := 0, size; bs < len(crypted); bs, be = bs+size, be+size {
		block.Decrypt(decrypted[bs:be], crypted[bs:be])
	}

	decrypted = padding(decrypted)
	return decrypted, nil
}

func AESECBDecryptWithPKCS7Padding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return _AESECBDecryptWithPadding(key, i, iv, PKCS7UnPadding)
}

var AESECBDecrypt = AESECBDecryptWithPKCS7Padding

func AESECBDecryptWithZeroPadding(key []byte, i interface{}, iv []byte) ([]byte, error) {
	return _AESECBDecryptWithPadding(key, i, iv, func(i []byte) []byte {
		return ZeroUnPadding(i)
	})
}

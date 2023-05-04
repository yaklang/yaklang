package codec

import (
	"crypto/cipher"
	"crypto/des"
	"fmt"
	"github.com/pkg/errors"
)

func desEnc(key []byte, data []byte, iv []byte, mode func(cipher.Block, []byte) cipher.BlockMode) ([]byte, error) {
	if iv == nil {
		iv = make([]byte, 8)
	} else {
		if len(iv)%8 != 0 {
			iv = ZeroPadding(iv, 8)
		}
	}
	if len(key)%8 != 0 {
		key = ZeroPadding(key, 8)
		if len(key) > 8 {
			key = key[:8]
		}
	}

	block, err := des.NewCipher(key)
	if err != nil {
		return nil, errors.Errorf("create cipher failed: %s", err)
	}

	if len(data)%8 != 0 {
		data = ZeroPadding(data, 8)
	}

	cbcMode := mode(block, iv)
	result := make([]byte, len(data))
	cbcMode.CryptBlocks(result, data)
	return result, nil
}

func desDec(key []byte, data []byte, iv []byte, mode func(cipher.Block, []byte) cipher.BlockMode) ([]byte, error) {
	if iv == nil {
		iv = make([]byte, 8)
	} else {
		if len(iv)%8 != 0 {
			iv = ZeroPadding(iv, 8)
		}
	}
	if len(key)%8 != 0 {
		key = ZeroPadding(key, 8)
		if len(key) > 8 {
			key = key[:8]
		}
	}

	block, err := des.NewCipher(key)
	if err != nil {
		return nil, errors.Errorf("create cipher failed: %s", err)
	}

	if len(data)%8 != 0 {
		data = ZeroPadding(data, 8)
	}

	cbcMode := mode(block, iv)
	result := make([]byte, len(data))
	cbcMode.CryptBlocks(result, data)
	return result, nil
}

func DESCBCEnc(key []byte, data []byte, iv []byte) ([]byte, error) {
	return desEnc(key, data, iv, cipher.NewCBCEncrypter)
}

func DESCBCDec(key, data, iv []byte) ([]byte, error) {
	return desDec(key, data, iv, cipher.NewCBCDecrypter)
}

func DESECBEnc(key []byte, data []byte) ([]byte, error) {
	blockSize := 8

	if len(key)%blockSize != 0 {
		key = ZeroPadding(key, blockSize)
		if len(key) > 8 {
			key = key[:8]
		}
	}

	block, err := des.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("DES ECB Error: %s", err)
	}
	if len(data)%blockSize != 0 {
		data = ZeroPadding(data, blockSize)
	}

	encrypted := make([]byte, len(data))
	for bs, be := 0, blockSize; bs < len(data); bs, be = bs+blockSize, be+blockSize {
		block.Encrypt(encrypted[bs:be], data[bs:be])
	}
	return encrypted, nil
}

func DESECBDec(key []byte, data []byte) ([]byte, error) {
	blockSize := 8

	if len(key)%blockSize != 0 {
		key = ZeroPadding(key, blockSize)
		if len(key) > 8 {
			key = key[:8]
		}
	}

	block, err := des.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("DES ECB Error: %s", err)
	}
	if len(data)%blockSize != 0 {
		data = ZeroPadding(data, blockSize)
	}

	decrypted := make([]byte, len(data))
	for bs, be := 0, blockSize; bs < len(data); bs, be = bs+blockSize, be+blockSize {
		block.Decrypt(decrypted[bs:be], data[bs:be])
	}
	return decrypted, nil
}

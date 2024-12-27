package codec

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"io"

	"github.com/pkg/errors"
)

/*
//AES GCM 加密后的payload shiro 1.4.2版本更换为了AES-GCM加密方式

	func AES_GCM_Encrypt(key []byte, Content []byte) string {
		block, _ := aes.NewCipher(key)
		nonce := make([]byte, 16)
		io.ReadFull(rand.Reader, nonce)
		aesgcm, _ := cipher.NewGCMWithNonceSize(block, 16)
		ciphertext := aesgcm.Seal(nil, nonce, Content, nil)
		return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...))
	}
*/
func AESGCMEncrypt(key []byte, data interface{}, nonceRaw []byte) ([]byte, error) {
	nonceSize := 12
	if len(nonceRaw) == 16 {
		nonceSize = 16
	}
	return AESGCMEncryptWithNonceSize(key, data, nonceRaw, nonceSize)
}

var AESGCMEncryptWithNonceSize16 = AESGCMEncrypt

func AESGCMEncryptWithNonceSize12(key []byte, data interface{}, nonceRaw []byte) ([]byte, error) {
	return AESGCMEncryptWithNonceSize(key, data, nonceRaw, 12)
}

func AESGCMEncryptWithNonceSize(key []byte, data interface{}, nonceRaw []byte, nonceSize int) ([]byte, error) {
	dataRaw := interfaceToBytes(data)

	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Errorf("create aes cipher failed: %s", err)
	}

	gcm, err := cipher.NewGCMWithNonceSize(c, nonceSize)
	if err != nil {
		return nil, errors.Errorf("create gcm failed: %s", err)
	}

	nonce := make([]byte, gcm.NonceSize())

	var randomNonce bool
	if len(nonceRaw) > 0 {
		copy(nonce, nonceRaw)
	} else {
		if _, err := io.ReadFull(cryptorand.Reader, nonce); err != nil {
			return nil, errors.Errorf("read nonce for aes_gcm failed: %s", err)
		}
		randomNonce = true
	}

	var buf bytes.Buffer
	if randomNonce {
		buf.Write(nonce)
	}
	buf.Write(gcm.Seal(nil, nonce, dataRaw, nil))
	return buf.Bytes(), nil
}

func AESGCMDecryptWithNonceSize(key []byte, data interface{}, nonceRaw []byte, nonceSize int) ([]byte, error) {
	dataRaw := interfaceToBytes(data)

	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Errorf("create aes cipher failed: %s", err)
	}

	gcm, err := cipher.NewGCMWithNonceSize(c, nonceSize)
	if err != nil {
		return nil, errors.Errorf("create gcm failed: %s", err)
	}

	// 兼容 nonce
	var nonce = make([]byte, nonceSize)
	if nonceRaw != nil {
		copy(nonce, nonceRaw)
	} else {
		if len(dataRaw) < nonceSize {
			return nil, errors.Errorf("nonce is empty, data[%v] is too short(cannot found nonce), ", StrConvQuoteHex(string(dataRaw)))
		}

		nonceFromData, encryptedData := dataRaw[:nonceSize], dataRaw[nonceSize:]
		copy(nonce, nonceFromData)
		if plain, err := gcm.Open(nil, nonce, encryptedData, nil); err == nil {
			return plain, nil
		}
	}
	return gcm.Open(nil, nonce, dataRaw, nil)
}

func AESGCMDecryptWithNonceSize12(key []byte, data interface{}, nonce []byte) ([]byte, error) {
	return AESGCMDecryptWithNonceSize(key, data, nonce, 12)
}

func AESGCMDecryptWithNonceSize16(key []byte, data interface{}, nonce []byte) ([]byte, error) {
	return AESGCMDecryptWithNonceSize(key, data, nonce, 16)
}

func AESGCMDecrypt(key []byte, data interface{}, nonce []byte) ([]byte, error) {
	nonceSize := 12
	if len(nonce) == 16 {
		nonceSize = 16
	}
	return AESGCMDecryptWithNonceSize(key, data, nonce, nonceSize)
}

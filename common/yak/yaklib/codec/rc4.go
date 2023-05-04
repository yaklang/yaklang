package codec

import (
	"crypto/rc4"
)

func RC4Decrypt(cipherKey []byte, cipherText []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(cipherKey)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(cipherText))
	cipher.XORKeyStream(plaintext, cipherText)
	return plaintext, nil
}

func RC4Encrypt(cipherKey []byte, plainText []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(cipherKey)
	if err != nil {
		return nil, err
	}
	ciphertext := make([]byte, len(plainText))
	cipher.XORKeyStream(ciphertext, plainText)
	return ciphertext, nil
}

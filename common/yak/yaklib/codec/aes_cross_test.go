package codec

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAESECBEncrypt(t *testing.T) {
	var key []byte
	var plain []byte
	var data []byte
	var iv []byte
	var encryptedRaw []byte
	var err error

	// pkcs7
	iv = []byte("aa")
	key = []byte("asdfasdfasdfasdf")
	plain = []byte(`abcHelloWorld` + "`" + `1123sdfasdasdf`)
	data = []byte(`4m7Z+sPRfCM0F77gJg9v6RAmt8hy9AqfAkhLtQiwZGw=`)
	encryptedRaw, err = AESEncryptECBWithPKCSPadding([]byte(key), plain, iv)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(encryptedRaw))
	if EncodeBase64(encryptedRaw) != string(data) {
		panic("aes ecb encrypt error")
	}
	raw, err := AESDecryptECBWithPKCSPadding(key, encryptedRaw, iv)
	if err != nil {
		panic(err)
	}
	if string(raw) != string(plain) {
		panic("aes failed")
	}

	// zeropadding
	iv = []byte("aa")
	key = []byte("asdfasdfasdfasdf")
	plain = []byte(`abcHelloWorld` + "`" + `1123sdfasdasdf`)
	data = []byte(`4m7Z+sPRfCM0F77gJg9v6aUrf1IbU9gZ8eemQZB8cCI=`)
	encryptedRaw, err = AESEncryptECBWithZeroPadding([]byte(key), plain, iv)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(encryptedRaw))
	if EncodeBase64(encryptedRaw) != string(data) {
		panic("aes ecb encrypt error")
	}
	raw, err = AESDecryptECBWithZeroPadding(key, encryptedRaw, iv)
	if err != nil {
		panic(err)
	}
	if string(raw) != string(plain) {
		panic("aes failed")
	}
}

func TestAESECBEncrypt2(t *testing.T) {
	var key []byte
	var plain []byte
	var data []byte
	var iv []byte
	var encryptedRaw []byte
	var err error

	// pkcs7
	iv = []byte("aa")
	key = []byte("asdfasdfasdfasdfaaaaaaaa")
	plain = []byte(`abcHelloWorld` + "`" + `1123sdfasdasdf`)
	data = []byte(`AT3zVDh1IuRnk3DfYboPHHPWnLjz5GSvZmx9gKUII0I=`)
	encryptedRaw, err = AESEncryptECBWithPKCSPadding([]byte(key), plain, iv)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(encryptedRaw))
	if EncodeBase64(encryptedRaw) != string(data) {
		panic("aes ecb encrypt error")
	}
	raw, err := AESDecryptECBWithPKCSPadding(key, encryptedRaw, iv)
	if err != nil {
		panic(err)
	}
	if string(raw) != string(plain) {
		panic("aes failed")
	}

	// zeropadding
	iv = []byte("aa")
	key = []byte("asdfasdfasdfasdfaaaaaaaa")
	plain = []byte(`abcHelloWorld` + "`" + `1123sdfasdasdf`)
	data = []byte(`AT3zVDh1IuRnk3DfYboPHGFQBhsctmch1PYxcnV7yM0=`)
	encryptedRaw, err = AESEncryptECBWithZeroPadding([]byte(key), plain, iv)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(encryptedRaw))
	if EncodeBase64(encryptedRaw) != string(data) {
		panic("eas ecb encrypt error")
	}
	raw, err = AESDecryptECBWithZeroPadding(key, encryptedRaw, iv)
	if err != nil {
		panic(err)
	}
	if string(raw) != string(plain) {
		panic("aes failed")
	}
}

func TestAESCBCEncrypt(t *testing.T) {
	var key []byte
	var plain []byte
	var data []byte
	var iv []byte
	var encryptedRaw []byte
	var err error

	// pkcs7
	iv = []byte("aabbccddaabbccdd")
	key = []byte("asdfasdfasdfasdf")
	plain = []byte(`abcHelloWorld` + "`" + `1123sdfasdasdf`)
	data = []byte(`4y+v7uadZUopc2N8rF2Yhsm3JgofMr4mCZEXH7xKdFM=`)
	encryptedRaw, err = AESEncryptCBCWithPKCSPadding([]byte(key), plain, iv)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(encryptedRaw))
	if EncodeBase64(encryptedRaw) != string(data) {
		panic(1)
	}
	raw, err := AESDecryptCBCWithPKCSPadding(key, encryptedRaw, iv)
	if err != nil {
		panic(err)
	}
	if string(raw) != string(plain) {
		panic(1)
	}

	// zeropadding
	iv = []byte("aabbccddaabbccdd")
	key = []byte("asdfasdfasdfasdf")
	plain = []byte(`abcHelloWorld` + "`" + `1123sdfasdasdf`)
	data = []byte(`4y+v7uadZUopc2N8rF2YhgmcAjyv28GlvoZaecovJtc=`)
	encryptedRaw, err = AESEncryptCBCWithZeroPadding([]byte(key), plain, iv)
	if err != nil {
		panic(1)
	}
	spew.Dump(EncodeBase64(encryptedRaw))
	if EncodeBase64(encryptedRaw) != string(data) {
		panic("eas ecb encrypt error")
	}
	raw, err = AESDecryptCBCWithZeroPadding(key, encryptedRaw, iv)
	if err != nil {
		panic(err)
	}
	if string(raw) != string(plain) {
		panic("aes failed")
	}
}

func TestAESCBCEncrypt2(t *testing.T) {
	var key []byte
	var plain []byte
	var data []byte
	var iv []byte
	var encryptedRaw []byte
	var err error

	// pkcs7
	iv = []byte("aabbccddaabbccddaaaaaaaa")
	key = []byte("asdfasdfasdfasdfaaaaaaaa")
	plain = []byte(`abcHelloWorld` + "`" + `1123sdfasdasdf`)
	data = []byte(`YvcnVzLeqrpiRZv8WO1poGdIhHv1bq/Yd2SwRbTnWhU=`)
	encryptedRaw, err = AESEncryptCBCWithPKCSPadding([]byte(key), plain, iv)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(encryptedRaw))
	if EncodeBase64(encryptedRaw) != string(data) {
		panic("enc failed")
	}
	raw, err := AESDecryptCBCWithPKCSPadding(key, encryptedRaw, iv)
	if err != nil {
		panic(err)
	}
	if string(raw) != string(plain) {
		panic("aes failed")
	}

	// zeropadding
	iv = []byte("aabbccddaabbccddaaaaaa")
	key = []byte("asdfasdfasdfasdfaaaaaaaa")
	plain = []byte(`abcHelloWorld` + "`" + `1123sdfasdasdf`)
	data = []byte(`YvcnVzLeqrpiRZv8WO1poDzAXEjuW+j4trTjpnoZoJg=`)
	encryptedRaw, err = AESEncryptCBCWithZeroPadding([]byte(key), plain, iv)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(encryptedRaw))
	if EncodeBase64(encryptedRaw) != string(data) {
		panic("aes encrypt error")
	}
	raw, err = AESDecryptCBCWithZeroPadding(key, encryptedRaw, iv)
	if err != nil {
		panic(err)
	}
	if string(raw) != string(plain) {
		panic("aes failed")
	}
}

func TestAESGCMEncrypt2(t *testing.T) {
	// gcm 无关 iv

	var key []byte
	var plain []byte
	var data []byte
	var iv []byte
	var encryptedRaw []byte
	var err error

	iv = []byte("aabbccddaabbccdd")
	key = []byte("abcdabcdabcdabcd")
	plain = []byte(`Hello`)
	data = []byte(`Wen/nKnDSTQwBtH2xaPWlk0sx9xN`)
	encryptedRaw, err = AESGCMEncrypt([]byte(key), plain, iv)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(encryptedRaw))
	if EncodeBase64(encryptedRaw) != string(data) {
		panic("enc failed")
	}
	raw, err := AESGCMDecrypt(key, encryptedRaw, iv)
	if err != nil {
		panic(err)
	}
	if string(raw) != string(plain) {
		spew.Dump(string(raw))
		panic("aes failed")
	}

	iv = []byte("aabbccddaabbccdd")
	key = []byte("abcdabcdabcdabcd")
	plain = []byte(`Hello`)
	data = []byte(`gWfY+bQaKBjZe/8puXW/t6PCiuB2`)
	encryptedRaw, err = AESGCMEncryptWithNonceSize12([]byte(key), plain, iv)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(encryptedRaw))
	if EncodeBase64(encryptedRaw) != string(data) {
		panic("enc failed")
	}
	raw, err = AESGCMDecryptWithNonceSize12(key, encryptedRaw, iv)
	if err != nil {
		panic(err)
	}
	if string(raw) != string(plain) {
		spew.Dump(string(raw))
		panic("aes failed")
	}
}

func TestAESWithPassphrase(t *testing.T) {
	rawData := RandBytes(10)
	password := RandBytes(10)
	salt := RandBytes(8)
	data := PKCS7Padding(rawData)
	cipher, err := AESEncWithPassphrase(password, data, salt, BytesToKeyMD5, "CBC")
	require.NoError(t, err)
	fmt.Println(cipher)

	plainText, err := AESDecWithPassphrase(password, cipher, salt, BytesToKeyMD5, "CBC")
	require.NoError(t, err)
	plainText = PKCS7UnPadding(plainText)
	fmt.Println(plainText)
	require.Equal(t, rawData, plainText)
}

package codec

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
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

func TestAESECBDecryptWithPKCS7Padding(t *testing.T) {
	key := RandBytes(16)

	testCases := []struct {
		name  string
		plain []byte
	}{
		{"empty", []byte{}},
		{"1 byte", []byte("a")},
		{"10 bytes", []byte("1234567890")},
		{"15 bytes", RandBytes(15)}, // Just under one block
		{"16 bytes", RandBytes(16)}, // Exactly one block
		{"17 bytes", RandBytes(17)}, // Just over one block
		{"32 bytes", RandBytes(32)}, // Exactly two blocks
		{"33 bytes", RandBytes(33)}, // Just over two blocks
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := AESDecryptECBWithPKCSPadding(key, tc.plain, nil)
			require.NoError(t, err, "decryption should succeed for %s", tc.name)
			_ = raw
		})
	}
}

// TestAESECBEncryptWithPKCS7Padding_KeyLengthValidation 测试 AES 加密函数对密钥长度的验证
// 验证：密钥长度必须是 16、24 或 32 字节，否则返回错误
func TestAESECBEncryptWithPKCS7Padding_KeyLengthValidation(t *testing.T) {
	plaintext := []byte("Hello Yak World!")

	testCases := []struct {
		name      string
		key       []byte
		shouldErr bool
	}{
		{"key 13 bytes (should return error)", []byte("aaaaaaaaaaaaa"), true},
		{"key 15 bytes (should return error)", []byte("aaaaaaaaaaaaaaa"), true},
		{"key 16 bytes (AES-128, valid)", []byte("aaaaaaaaaaaaaaaa"), false},
		{"key 17 bytes (should return error)", []byte("aaaaaaaaaaaaaaaaa"), true},
		{"key 23 bytes (should return error)", make([]byte, 23), true},
		{"key 24 bytes (AES-192, valid)", make([]byte, 24), false},
		{"key 25 bytes (should return error)", make([]byte, 25), true},
		{"key 31 bytes (should return error)", make([]byte, 31), true},
		{"key 32 bytes (AES-256, valid)", make([]byte, 32), false},
		{"key 33 bytes (should return error)", make([]byte, 33), true},
		{"key 40 bytes (should return error)", make([]byte, 40), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 加密
			encrypted, err := AESEncryptECBWithPKCSPadding(tc.key, plaintext, nil)
			if tc.shouldErr {
				require.Error(t, err, "encryption should fail for %s", tc.name)
				require.Contains(t, err.Error(), "AES key length must be 16, 24, or 32 bytes", "error message should mention key length requirement")
				require.Nil(t, encrypted, "encrypted data should be nil when encryption fails")
				return
			}
			require.NoError(t, err, "encryption should succeed for %s", tc.name)
			require.NotNil(t, encrypted, "encrypted data should not be nil for %s", tc.name)
			require.Greater(t, len(encrypted), 0, "encrypted data should not be empty for %s", tc.name)

			// 解密
			decrypted, err := AESDecryptECBWithPKCSPadding(tc.key, encrypted, nil)
			require.NoError(t, err, "decryption should succeed for %s", tc.name)
			require.NotNil(t, decrypted, "decrypted data should not be nil for %s", tc.name)
			require.Equal(t, plaintext, decrypted, "decrypted data should match original plaintext for %s", tc.name)
		})
	}
}

// TestAESECBEncryptWithPKCS7Padding_VariousPlaintextLengths 测试不同长度的明文加密
func TestAESECBEncryptWithPKCS7Padding_VariousPlaintextLengths(t *testing.T) {
	key := []byte("aaaaaaaaaaaaaaaa") // 16 bytes for AES-128

	testCases := []struct {
		name      string
		plaintext []byte
	}{
		{"empty", []byte{}},
		{"1 byte", []byte("a")},
		{"10 bytes", []byte("1234567890")},
		{"15 bytes", []byte("123456789012345")},   // Just under one block
		{"16 bytes", []byte("1234567890123456")},  // Exactly one block
		{"17 bytes", []byte("12345678901234567")}, // Just over one block
		{"32 bytes", make([]byte, 32)},            // Exactly two blocks
		{"33 bytes", make([]byte, 33)},            // Just over two blocks
		{"100 bytes", make([]byte, 100)},          // Multiple blocks
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 加密
			encrypted, err := AESEncryptECBWithPKCSPadding(key, tc.plaintext, nil)
			require.NoError(t, err, "encryption should succeed for %s", tc.name)
			require.NotNil(t, encrypted, "encrypted data should not be nil for %s", tc.name)
			require.Greater(t, len(encrypted), 0, "encrypted data should not be empty for %s", tc.name)

			// 验证加密后的数据长度是块大小的倍数
			require.Equal(t, 0, len(encrypted)%16, "encrypted data length should be multiple of block size (16) for %s", tc.name)

			// 解密
			decrypted, err := AESDecryptECBWithPKCSPadding(key, encrypted, nil)
			require.NoError(t, err, "decryption should succeed for %s", tc.name)
			require.NotNil(t, decrypted, "decrypted data should not be nil for %s", tc.name)
			require.Equal(t, tc.plaintext, decrypted, "decrypted data should match original plaintext for %s", tc.name)
		})
	}
}

// TestAESKeyLengthValidation 验证密钥长度验证功能
// 这个测试验证：密钥长度必须是 16、24 或 32 字节，否则会返回错误
func TestAESKeyLengthValidation(t *testing.T) {
	plaintext := []byte("Hello Yak World!")

	// 测试无效密钥长度（应该返回错误）
	invalidKeyCases := []struct {
		name string
		key  []byte
	}{
		{"empty key", []byte{}},
		{"1 byte key", []byte("a")},
		{"13 bytes key", []byte("aaaaaaaaaaaaa")},
		{"15 bytes key", make([]byte, 15)},
		{"17 bytes key", make([]byte, 17)},
		{"23 bytes key", make([]byte, 23)},
		{"25 bytes key", make([]byte, 25)},
		{"31 bytes key", make([]byte, 31)},
		{"33 bytes key", make([]byte, 33)},
		{"40 bytes key", make([]byte, 40)},
		{"100 bytes key", make([]byte, 100)},
	}

	for _, tc := range invalidKeyCases {
		t.Run("invalid_"+tc.name, func(t *testing.T) {
			// 验证加密应该返回错误
			encrypted, err := AESEncryptECBWithPKCSPadding(tc.key, plaintext, nil)
			require.Error(t, err, "encryption should fail for invalid key length: %s", tc.name)
			require.Contains(t, err.Error(), "AES key length must be 16, 24, or 32 bytes", "error message should mention key length requirement")
			require.Nil(t, encrypted, "encrypted data should be nil when encryption fails")

			// 验证解密也应该返回错误
			decrypted, err := AESDecryptECBWithPKCSPadding(tc.key, []byte("dummy ciphertext"), nil)
			require.Error(t, err, "decryption should fail for invalid key length: %s", tc.name)
			require.Contains(t, err.Error(), "AES key length must be 16, 24, or 32 bytes", "error message should mention key length requirement")
			require.Nil(t, decrypted, "decrypted data should be nil when decryption fails")
		})
	}

	// 测试有效密钥长度（应该成功）
	validKeyCases := []struct {
		name string
		key  []byte
	}{
		{"16 bytes key (AES-128)", make([]byte, 16)},
		{"24 bytes key (AES-192)", make([]byte, 24)},
		{"32 bytes key (AES-256)", make([]byte, 32)},
	}

	for _, tc := range validKeyCases {
		t.Run("valid_"+tc.name, func(t *testing.T) {
			// 验证加密应该成功
			encrypted, err := AESEncryptECBWithPKCSPadding(tc.key, plaintext, nil)
			require.NoError(t, err, "encryption should succeed for valid key length: %s", tc.name)
			require.NotNil(t, encrypted, "encrypted data should not be nil for %s", tc.name)

			// 验证解密也应该成功
			decrypted, err := AESDecryptECBWithPKCSPadding(tc.key, encrypted, nil)
			require.NoError(t, err, "decryption should succeed for valid key length: %s", tc.name)
			require.NotNil(t, decrypted, "decrypted data should not be nil for %s", tc.name)
			require.Equal(t, plaintext, decrypted, "decrypted data should match original plaintext for %s", tc.name)
		})
	}
}

// TestAESGCMDecryptErrorHandling 测试 AES GCM 模式的错误处理
func TestAESGCMDecryptErrorHandling(t *testing.T) {
	plaintext := []byte("Hello Yak World! This is a test message for GCM mode.")
	key := []byte("1234567890123456")    // 16 bytes key for AES-128
	validNonce := []byte("aabbccddaabb") // 12 bytes nonce

	// 1. 正常加密解密应该成功
	encrypted, err := AESGCMEncryptWithNonceSize12(key, plaintext, validNonce)
	require.NoError(t, err, "encryption should succeed")
	require.NotNil(t, encrypted, "encrypted data should not be nil")

	decrypted, err := AESGCMDecryptWithNonceSize12(key, encrypted, validNonce)
	require.NoError(t, err, "decryption should succeed with correct key and nonce")
	require.Equal(t, plaintext, decrypted, "decrypted data should match original plaintext")

	// 2. 使用错误的密钥解密应该返回错误（不是损坏的内容）
	wrongKey := []byte("wrongkey12345678") // 16 bytes but wrong key
	decrypted, err = AESGCMDecryptWithNonceSize12(wrongKey, encrypted, validNonce)
	require.Error(t, err, "decryption should fail with wrong key")
	require.Nil(t, decrypted, "decrypted data should be nil when decryption fails")
	// 验证错误不是 padding 相关的，而是 GCM 认证失败
	require.NotContains(t, err.Error(), "padding", "error should not be about padding")
	require.NotContains(t, err.Error(), "PKCS", "error should not be about PKCS padding")
	// GCM 的错误信息可能包含 "message authentication failed" 或其他认证相关的错误
	// 关键是要确保这不是 padding 错误，而是真正的解密/认证错误

	// 3. 使用错误的 nonce 解密应该返回错误
	wrongNonce := []byte("wrongnonce12") // 12 bytes but wrong nonce
	decrypted, err = AESGCMDecryptWithNonceSize12(key, encrypted, wrongNonce)
	require.Error(t, err, "decryption should fail with wrong nonce")
	require.Nil(t, decrypted, "decrypted data should be nil when decryption fails")
	require.NotContains(t, err.Error(), "padding", "error should not be about padding")
	require.NotContains(t, err.Error(), "PKCS", "error should not be about PKCS padding")
	// GCM 的错误应该是认证失败，而不是 padding 问题

	// 4. 修改密文后解密应该返回错误（GCM 有完整性校验）
	modifiedCiphertext := make([]byte, len(encrypted))
	copy(modifiedCiphertext, encrypted)
	// 修改密文的最后一个字节
	if len(modifiedCiphertext) > 0 {
		modifiedCiphertext[len(modifiedCiphertext)-1] ^= 0x01
	}
	decrypted, err = AESGCMDecryptWithNonceSize12(key, modifiedCiphertext, validNonce)
	require.Error(t, err, "decryption should fail with modified ciphertext")
	require.Nil(t, decrypted, "decrypted data should be nil when decryption fails")
	require.NotContains(t, err.Error(), "padding", "error should not be about padding")
	require.NotContains(t, err.Error(), "PKCS", "error should not be about PKCS padding")
	// GCM 的错误应该是认证失败，而不是 padding 问题

	// 5. 使用错误的密钥长度应该返回错误（在创建 cipher 时就会失败）
	invalidKey := []byte("short") // 5 bytes, invalid for AES
	_, err = AESGCMEncryptWithNonceSize12(invalidKey, plaintext, validNonce)
	require.Error(t, err, "encryption should fail with invalid key length")
	require.Contains(t, err.Error(), "cipher", "error should mention cipher creation failure")

	// 6. 使用无效的密文长度（太短，无法包含 nonce + ciphertext + tag）
	tooShortCiphertext := []byte("short")
	decrypted, err = AESGCMDecryptWithNonceSize12(key, tooShortCiphertext, validNonce)
	require.Error(t, err, "decryption should fail with too short ciphertext")
	require.Nil(t, decrypted, "decrypted data should be nil when decryption fails")
	// 这个错误可能是关于数据长度或 nonce 的，不是 padding

	// 7. 验证 GCM 模式不会像 ECB 那样返回损坏的内容
	// 在 ECB 模式下，使用错误密钥可能会返回一些看起来像数据的内容（虽然损坏）
	// 但在 GCM 模式下，应该直接返回错误
	encrypted2, err := AESGCMEncryptWithNonceSize12(key, plaintext, validNonce)
	require.NoError(t, err)

	// 尝试用错误密钥解密，应该得到错误而不是损坏的数据
	wrongKey2 := []byte("anotherwrongkey1") // 16 bytes but wrong (确保长度正确)
	decrypted, err = AESGCMDecryptWithNonceSize12(wrongKey2, encrypted2, validNonce)
	require.Error(t, err, "decryption with wrong key should return error, not corrupted data")
	require.Nil(t, decrypted, "GCM should return nil on error, not corrupted data")
	// GCM 的错误可能是 "cipher: message authentication failed" 或其他格式
	// 但关键是不应该包含 padding 相关的错误
	require.NotContains(t, err.Error(), "padding", "error should not be about padding")
	require.NotContains(t, err.Error(), "PKCS", "error should not be about PKCS padding")
}

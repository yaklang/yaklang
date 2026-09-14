package codec

import (
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func TestDESCBCDec(t *testing.T) {
	origin, err := DESEncryptCBCWithZeroPadding(ZeroPadding([]byte("test"), 8), []byte("asdfasdfasdfsdfasdf"), nil)
	if err != nil {
		panic(err)
	}
	println(StrConvQuoteHex(string(origin)))

	data, err := DESDecryptCBCWithZeroPadding(ZeroPadding([]byte("test"), 8), origin, nil)
	if err != nil {
		panic(err)
	}
	println(StrConvQuoteHex(string(data)))
}

func TestDesECB(t *testing.T) {
	bytes, err := DESECBEnc(ZeroPadding([]byte(`abc`), 8), []byte(`abc`))
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(bytes))

	origin, err := DESECBDec(ZeroPadding([]byte(`abc`), 8), bytes)
	if err != nil {
		panic(err)
	}
	spew.Dump(origin)
}

func TestTripleDES_CBC(t *testing.T) {
	ede2Key := []byte("example key 1234")
	var tripleDESKey []byte
	tripleDESKey = append(tripleDESKey, ede2Key[:16]...)
	tripleDESKey = append(tripleDESKey, ede2Key[:8]...)

	plainText := "abc"

	bytes, err := TripleDESEncryptCBCWithZeroPadding(tripleDESKey, []byte(plainText), nil)
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(bytes))

	origin, err := TripleDESDecryptCBCWithZeroPadding(tripleDESKey, bytes, nil)
	if err != nil {
		panic(err)
	}
	spew.Dump(origin)

	if strings.Trim(string(origin), "\x00") != plainText {
		panic("not expected")
	}
}

func TestTripleDES_ECB(t *testing.T) {
	ede2Key := []byte("example key 1234")
	var tripleDESKey []byte
	tripleDESKey = append(tripleDESKey, ede2Key[:16]...)
	tripleDESKey = append(tripleDESKey, ede2Key[:8]...)
	plainText := "abc"

	t.Run("enc", func(t *testing.T) {

		bytes, err := TripleDES_ECBEnc(tripleDESKey, []byte(plainText))
		require.NoError(t, err)

		origin, err := TripleDESDecFactory(ZeroUnPadding, ECB)(tripleDESKey, bytes, nil)
		require.NoError(t, err)
		spew.Dump(origin)
		require.Equal(t, plainText, strings.Trim(string(origin), "\x00"))
	})

	t.Run("dec", func(t *testing.T) {
		bytes, err := TripleDESEncFactory(ZeroPadding, ECB)(tripleDESKey, []byte(plainText), nil)
		require.NoError(t, err)
		spew.Dump(EncodeBase64(bytes))

		origin, err := TripleDES_ECBDec(tripleDESKey, bytes)
		require.NoError(t, err)
		spew.Dump(origin)
		require.Equal(t, plainText, strings.Trim(string(origin), "\x00"))
	})

}

// TestDESEncryptionModes 验证 DES 各种模式的加密行为
// 流模式（CTR、CFB、OFB）：密文长度应等于明文长度（无 padding）
// 块模式（CBC、ECB）：密文长度应为块大小的倍数（有 padding）
func TestDESEncryptionModes(t *testing.T) {
	key := []byte("12345678") // 8 bytes key
	iv := []byte("abcdabcd")  // 8 bytes IV

	testCases := []struct {
		name          string
		mode          string
		plaintext     []byte
		expectPadding bool
	}{
		{"CTR/1byte", CTR, []byte("a"), false},
		{"CTR/5bytes", CTR, []byte("hello"), false},
		{"CTR/8bytes", CTR, []byte("12345678"), false},
		{"CTR/9bytes", CTR, []byte("123456789"), false},
		{"CTR/17bytes", CTR, []byte("12345678901234567"), false},
		{"CFB/5bytes", CFB, []byte("hello"), false},
		{"CFB/13bytes", CFB, []byte("Hello, World!"), false},
		{"CFB/23bytes", CFB, []byte("12345678901234567890123"), false},
		{"OFB/7bytes", OFB, []byte("testing"), false},
		{"OFB/11bytes", OFB, []byte("hello world"), false},
		{"OFB/25bytes", OFB, []byte("1234567890123456789012345"), false},
		{"CBC/5bytes", CBC, []byte("hello"), true},
		{"CBC/8bytes", CBC, []byte("12345678"), true},
		{"CBC/9bytes", CBC, []byte("123456789"), true},
		{"ECB/5bytes", ECB, []byte("hello"), true},
		{"ECB/8bytes", ECB, []byte("12345678"), true},
		{"ECB/9bytes", ECB, []byte("123456789"), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ciphertext, err := DESEnc(key, tc.plaintext, iv, tc.mode)
			require.NoError(t, err, "encryption should succeed for %s", tc.mode)
			require.NotNil(t, ciphertext, "ciphertext should not be nil for %s", tc.mode)

			if tc.expectPadding {
				require.Equal(t, 0, len(ciphertext)%8,
					"Block mode %s: ciphertext length should be multiple of block size (8 bytes)", tc.mode)
				if len(tc.plaintext)%8 != 0 {
					require.Greater(t, len(ciphertext), len(tc.plaintext),
						"Block mode %s: ciphertext should be longer than plaintext due to padding", tc.mode)
				}
			} else {
				require.Equal(t, len(tc.plaintext), len(ciphertext),
					"Stream mode %s: ciphertext length must equal plaintext length (no padding)", tc.mode)
			}

			decrypted, err := DESDec(key, ciphertext, iv, tc.mode)
			require.NoError(t, err, "decryption should succeed for %s", tc.mode)
			require.NotNil(t, decrypted, "decrypted data should not be nil for %s", tc.mode)

			if !tc.expectPadding {
				require.Equal(t, len(tc.plaintext), len(decrypted),
					"Stream mode %s: decrypted length should equal plaintext length", tc.mode)
				require.Equal(t, tc.plaintext, decrypted,
					"Stream mode %s: decrypted data should match original plaintext", tc.mode)
			} else {
				// 块模式：DESDec 不做 unpadding，解密后长度等于密文长度
				require.Equal(t, len(ciphertext), len(decrypted),
					"Block mode %s: DESDec doesn't unpadding, so decrypted length equals ciphertext length", tc.mode)
			}
		})
	}
}

// TestTripleDESEncryptionModes 验证 TripleDES 各种模式的加密行为
// 流模式（CTR、CFB、OFB）：密文长度应等于明文长度（无 padding）
// 块模式（CBC、ECB）：密文长度应为块大小的倍数（有 padding）
func TestTripleDESEncryptionModes(t *testing.T) {
	key := []byte("123456789012345678901234") // 24 bytes key
	iv := []byte("abcdabcd")                  // 8 bytes IV

	testCases := []struct {
		name          string
		mode          string
		plaintext     []byte
		expectPadding bool
	}{
		{"CTR/1byte", CTR, []byte("a"), false},
		{"CTR/5bytes", CTR, []byte("hello"), false},
		{"CTR/8bytes", CTR, []byte("12345678"), false},
		{"CTR/9bytes", CTR, []byte("123456789"), false},
		{"CTR/17bytes", CTR, []byte("12345678901234567"), false},
		{"CFB/5bytes", CFB, []byte("hello"), false},
		{"CFB/13bytes", CFB, []byte("Hello, World!"), false},
		{"CFB/23bytes", CFB, []byte("12345678901234567890123"), false},
		{"OFB/7bytes", OFB, []byte("testing"), false},
		{"OFB/11bytes", OFB, []byte("hello world"), false},
		{"OFB/25bytes", OFB, []byte("1234567890123456789012345"), false},
		{"CBC/5bytes", CBC, []byte("hello"), true},
		{"CBC/8bytes", CBC, []byte("12345678"), true},
		{"CBC/9bytes", CBC, []byte("123456789"), true},
		{"ECB/5bytes", ECB, []byte("hello"), true},
		{"ECB/8bytes", ECB, []byte("12345678"), true},
		{"ECB/9bytes", ECB, []byte("123456789"), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ciphertext, err := TripleDesEnc(key, tc.plaintext, iv, tc.mode)
			require.NoError(t, err, "encryption should succeed for %s", tc.mode)
			require.NotNil(t, ciphertext, "ciphertext should not be nil for %s", tc.mode)

			if tc.expectPadding {
				require.Equal(t, 0, len(ciphertext)%8,
					"Block mode %s: ciphertext length should be multiple of block size (8 bytes)", tc.mode)
				if len(tc.plaintext)%8 != 0 {
					require.Greater(t, len(ciphertext), len(tc.plaintext),
						"Block mode %s: ciphertext should be longer than plaintext due to padding", tc.mode)
				}
			} else {
				require.Equal(t, len(tc.plaintext), len(ciphertext),
					"Stream mode %s: ciphertext length must equal plaintext length (no padding)", tc.mode)
			}

			decrypted, err := TripleDesDec(key, ciphertext, iv, tc.mode)
			require.NoError(t, err, "decryption should succeed for %s", tc.mode)
			require.NotNil(t, decrypted, "decrypted data should not be nil for %s", tc.mode)

			if !tc.expectPadding {
				require.Equal(t, len(tc.plaintext), len(decrypted),
					"Stream mode %s: decrypted length should equal plaintext length", tc.mode)
				require.Equal(t, tc.plaintext, decrypted,
					"Stream mode %s: decrypted data should match original plaintext", tc.mode)
			} else {
				// 块模式：TripleDesDec 不做 unpadding，解密后长度等于密文长度
				require.Equal(t, len(ciphertext), len(decrypted),
					"Block mode %s: TripleDesDec doesn't unpadding, so decrypted length equals ciphertext length", tc.mode)
			}
		})
	}
}

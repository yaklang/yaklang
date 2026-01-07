package codec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAESEncryptionModes 试验证 AES 各种模式的加密行为
// 流模式（CTR、CFB、OFB）：密文长度应等于明文长度（无 padding）
// 块模式（CBC、ECB）：密文长度应为块大小的倍数（有 padding）
func TestAESEncryptionModes(t *testing.T) {
	key := []byte("1234567890123456") // 16 bytes key for AES-128
	iv := []byte("abcdabcdabcdabcd")  // 16 bytes IV

	testCases := []struct {
		name          string
		mode          string
		plaintext     []byte
		expectPadding bool // 是否期望有 padding
		description   string
	}{
		// 流模式测试 - 不应该有 padding
		{"CTR/1byte", CTR, []byte("a"), false, "CTR mode with 1 byte"},
		{"CTR/5bytes", CTR, []byte("hello"), false, "CTR mode with 5 bytes (hello)"},
		{"CTR/10bytes", CTR, []byte("1234567890"), false, "CTR mode with 10 bytes"},
		{"CTR/15bytes", CTR, []byte("123456789012345"), false, "CTR mode just under one block"},
		{"CTR/16bytes", CTR, []byte("1234567890123456"), false, "CTR mode exactly one block"},
		{"CTR/17bytes", CTR, []byte("12345678901234567"), false, "CTR mode just over one block"},
		{"CTR/23bytes", CTR, []byte("12345678901234567890123"), false, "CTR mode 23 bytes"},
		{"CTR/33bytes", CTR, []byte("123456789012345678901234567890123"), false, "CTR mode 33 bytes"},
		{"CTR/100bytes", CTR, make([]byte, 100), false, "CTR mode 100 bytes"},

		{"CFB/5bytes", CFB, []byte("hello"), false, "CFB mode with 5 bytes"},
		{"CFB/13bytes", CFB, []byte("Hello, World!"), false, "CFB mode with 13 bytes"},
		{"CFB/23bytes", CFB, []byte("12345678901234567890123"), false, "CFB mode 23 bytes"},

		{"OFB/7bytes", OFB, []byte("testing"), false, "OFB mode with 7 bytes"},
		{"OFB/11bytes", OFB, []byte("hello world"), false, "OFB mode with 11 bytes"},
		{"OFB/25bytes", OFB, []byte("1234567890123456789012345"), false, "OFB mode 25 bytes"},

		// 块模式测试 - 应该有 padding
		{"CBC/5bytes", CBC, []byte("hello"), true, "CBC mode with 5 bytes"},
		{"CBC/10bytes", CBC, []byte("1234567890"), true, "CBC mode with 10 bytes"},
		{"CBC/15bytes", CBC, []byte("123456789012345"), true, "CBC mode just under one block"},

		{"ECB/5bytes", ECB, []byte("hello"), true, "ECB mode with 5 bytes"},
		{"ECB/10bytes", ECB, []byte("1234567890"), true, "ECB mode with 10 bytes"},
		{"ECB/17bytes", ECB, []byte("12345678901234567"), true, "ECB mode just over one block"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 加密
			ciphertext, err := AESEnc(key, tc.plaintext, iv, tc.mode)
			require.NoError(t, err, "encryption should succeed for %s", tc.description)
			require.NotNil(t, ciphertext, "ciphertext should not be nil for %s", tc.description)

			if tc.expectPadding {
				// 块模式：密文长度应该是 16 的倍数，且通常大于明文长度
				require.Equal(t, 0, len(ciphertext)%16,
					"Block mode %s: ciphertext length should be multiple of block size (16 bytes)", tc.mode)
				if len(tc.plaintext)%16 != 0 {
					require.Greater(t, len(ciphertext), len(tc.plaintext),
						"Block mode %s: ciphertext should be longer than plaintext due to padding", tc.mode)
				}
			} else {
				// 流模式：密文长度应严格等于明文长度（无 padding）
				require.Equal(t, len(tc.plaintext), len(ciphertext),
					"Stream mode %s: ciphertext length must equal plaintext length (no padding)", tc.mode)
			}

			// 解密
			decrypted, err := AESDec(key, ciphertext, iv, tc.mode)
			require.NoError(t, err, "decryption should succeed for %s", tc.description)
			require.NotNil(t, decrypted, "decrypted data should not be nil for %s", tc.description)

			if !tc.expectPadding {
				// 流模式：解密后长度应等于明文长度，内容应完全一致
				require.Equal(t, len(tc.plaintext), len(decrypted),
					"Stream mode %s: decrypted length should equal plaintext length", tc.mode)
				require.Equal(t, tc.plaintext, decrypted,
					"Stream mode %s: decrypted data should match original plaintext", tc.mode)
			} else {
				// 块模式：解密后可能包含 padding（取决于 AESDec 实现）
				// 当前 AESDec 不做 unpadding，所以长度等于密文长度
				require.Equal(t, len(ciphertext), len(decrypted),
					"Block mode %s: AESDec doesn't unpadding, so decrypted length equals ciphertext length", tc.mode)
			}
		})
	}
}

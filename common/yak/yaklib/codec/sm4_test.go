package codec

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/gmsm/sm4"
	"github.com/yaklang/yaklang/common/gmsm/sm4/padding"
	"github.com/yaklang/yaklang/common/log"
)

func TestSM4(t *testing.T) {
	var count = 0
	for {
		count++
		if count > 20 {
			return
		}

		key := []byte("2345223412341234")
		target := "asdfasdfjojoq234tjoq3reaasdfasdfasdfasdfasdfjtopqd"
		for method, item := range map[string][]SymmetricCryptFunc{
			"sm4_cbc": {SM4EncryptCBCWithPKCSPadding, SM4DecryptCBCWithPKCSPadding},
			"sm4_cfb": {SM4EncryptCFBWithPKCSPadding, SM4DecryptCFBWithPKCSPadding},
			"sm4_ecb": {SM4EncryptECBWithPKCSPadding, SM4DecryptECBWithPKCSPadding},
			"sm4_ofb": {SM4EncryptOFBWithPKCSPadding, SM4DecryptOFBWithPKCSPadding},
			"sm4_gcm": {SM4GCMEnc, SM4GCMDec},
			"aes_gcm": {AESGCMEncrypt, AESGCMDecrypt},
			"aes_cbc": {AESEncryptCBCWithPKCSPadding, AESDecryptCBCWithPKCSPadding},
		} {
			log.Infof("start to test: %v", method)
			enc := item[0]
			dec := item[1]
			tm, err := enc(key, target, nil)
			if err != nil {
				log.Error(err)
				t.FailNow()
			}
			log.Infof("enc %v finished: %v", method, StrConvQuoteHex(string(tm)))
			origin, err := dec(key, tm, nil)
			if err != nil {
				log.Error(err)
				t.FailNow()
			}
			log.Infof("dec %v finished: %v", method, StrConvQuoteHex(string(origin)))

			if target != string(origin) {
				log.Errorf("failed for %#v", method)
				t.FailNow()
			}
		}
	}
}

func TestECB(t *testing.T) {
	// reM7Nv3xlWHFPhh6iomFgcrFqPg5VGegvbiyD3rKCQvuXy+kUP4satgd1c/4t1pZ23v8wXIEY14eMXPc9sXivT+jfR7iNOknzTHjZCEVBvkm0EKa2WVrIb9665ze8yRm
	data, _ := DecodeBase64("reM7Nv3xlWHFPhh6iomFgcrFqPg5VGegvbiyD3rKCQvuXy+kUP4satgd1c/4t1pZ23v8wXIEY14eMXPc9sXivT+jfR7iNOknzTHjZCEVBvkm0EKa2WVrIb9665ze8yRm")
	var result, err = SM4DecryptECBWithPKCSPadding([]byte("11HDESaAhiHHugDz"), data, []byte(`UISwD9fW6cFh9SNS`))
	if err != nil {
		panic(err)
	}
	if result == nil {
		panic("EMPTY RESULT")
	}
	spew.Dump(result)
}

func TestSM4ECBDec(t *testing.T) {
	var raw, _ = DecodeBase64(`Kh1Ou151chL8Ondn6l5hgA==`)
	results, err := SM4DecryptECBWithPKCSPadding([]byte(`1234123412341234`), raw, nil)
	if err != nil {
		panic(err)
	}
	if string(results) != "asdfasd" {
		panic("SM4ECB FAILED")
	}

	raw, _ = DecodeBase64(`jw/eNRHMJAZZUsEV/Ue1rAQ/H/rvsFIXLDpbnGM9kYI=`)
	results, err = SM4DecryptECBWithPKCSPadding([]byte(`1234123412341234`), raw, nil)
	if err != nil {
		panic(err)
	}
	if string(results) != "asdfasdfasdfasdf" {
		panic("SM4ECB FAILED")
	}
}

func TestPadding(t *testing.T) {
	testData, _ := DecodeBase64(`r8+ZCQ5kPYBsVzlnWcwF2T4hm94cfWGr/B9sf5I9GoiJfm6w46gHvB7ua7hle7u3zfQlTB0g0ovoWmU583Ssl+u5mY2AOyOFJPn71HnKWaCLwrsDpOBEO2rHSRSdob4a`)
	for _, i := range []string{
		"abcd",
		"abcdabcdabcdabcd",
		"abcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd",
		"abcdabcd",
		"abcdabcdabcdabcdabcd",
		string(testData),
	} {
		var result = PKCS5Padding([]byte(i), 8)
		i2 := PKCS5UnPadding(result)
		if len(i2) != len(i) {
			spew.Dump(result)
			spew.Dump(i2)
			spew.Dump(i)
			panic(1)
		}
		var i3 = PKCS5UnPadding([]byte(i))
		if len(i3) != len(i) {
			spew.Dump(i3)
			spew.Dump([]byte(i))
			panic("Unsafe Padding5")
		}
		if i3 == nil {
			panic(1)
		}
	}
	//
	//for _, i := range []string{
	//	"abcd",
	//	"abcdabcdabcdabcd",
	//	"abcdabcd",
	//	"abcdabcdabcdabcdabcd",
	//} {
	//	var result = ZeroPadding([]byte(i), 16)
	//	spew.Dump(result)
	//}

	for _, i := range []string{
		"abcd",
		"abcdabcdabcdabcd",
		"abcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd",
		"abcdabcd",
		"abcdabcdabcdabcdabcd",
		string(testData),
	} {
		var result = PKCS7Padding([]byte(i))
		i2 := PKCS7UnPadding(result)
		if len(i2) != len(i) {
			panic(1)
		}
		var i3 = PKCS7UnPadding([]byte(i))
		if len(i3) != len(i) {
			panic("Unsafe Padding7")
		}
		if i3 == nil {
			panic(1)
		}
	}
}

func TestSM4GCMStream(t *testing.T) {
	key := []byte("1234567890abcdef")
	data := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10}
	IV := make([]byte, sm4.BlockSize)
	testA := [][]byte{ // the length of the A can be random
		[]byte{},
		[]byte{0x01, 0x23, 0x45, 0x67, 0x89},
		[]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10},
	}
	for _, A := range testA {
		plain := bytes.NewReader(data)
		var gcmCipher bytes.Buffer
		T, err := sm4.GCMEncryptStream(key, IV, A, padding.NewPKCSPaddingReader(plain, 16), &gcmCipher)
		require.NoError(t, err)
		fmt.Printf("gcmMsg = %x\n", gcmCipher.Bytes())

		var gcmPlain bytes.Buffer
		T_, err := sm4.GCMDecryptStream(key, IV, A, &gcmCipher, padding.NewPKCSPaddingWriter(&gcmPlain, 16))
		require.NoError(t, err)
		fmt.Printf("gcmDec = %x\n", gcmPlain.Bytes())
		require.Equal(t, T, T_, "authentication not successed")
		require.Equal(t, data, gcmPlain.Bytes(), "decrypt fail")

		//Failed Test : if we input the different A , that will be a falied result.
		A = []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd}
		plain = bytes.NewReader(data)
		gcmCipher = bytes.Buffer{}
		T, err = sm4.GCMEncryptStream(key, IV, A, padding.NewPKCSPaddingReader(plain, 16), &gcmCipher)
		require.NoError(t, err)
		require.NotEqual(t, T, T_, "authentication tag should not equal")
	}

}

func TestSM4GCMStreamZero(t *testing.T) {
	key := []byte("1234567890abcdef")
	data := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0x1}
	IV := make([]byte, sm4.BlockSize)
	testA := [][]byte{ // the length of the A can be random
		[]byte{},
		[]byte{0x01, 0x23, 0x45, 0x67, 0x89},
		[]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10},
	}
	for _, A := range testA {
		plain := bytes.NewReader(data)
		var gcmCipher bytes.Buffer
		T, err := sm4.GCMEncryptStream(key, IV, A, padding.NewZeroPaddingReader(plain, 16), &gcmCipher)
		require.NoError(t, err)
		fmt.Printf("gcmMsg = %x\n", gcmCipher.Bytes())

		var gcmPlain bytes.Buffer
		T_, err := sm4.GCMDecryptStream(key, IV, A, &gcmCipher, padding.NewZeroPaddingWriter(&gcmPlain, 16))
		require.NoError(t, err)
		fmt.Printf("gcmDec = %x\n", gcmPlain.Bytes())
		require.Equal(t, T, T_, "authentication not successed")
		require.Equal(t, data, gcmPlain.Bytes(), "decrypt fail")

		//Failed Test : if we input the different A , that will be a falied result.
		A = []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd}
		plain = bytes.NewReader(data)
		gcmCipher = bytes.Buffer{}
		T, err = sm4.GCMEncryptStream(key, IV, A, padding.NewZeroPaddingReader(plain, 16), &gcmCipher)
		require.NoError(t, err)
		require.NotEqual(t, T, T_, "authentication tag should not equal")
	}
}

// TestSM4APIConsistency 测试 SM4 API 的一致性
func TestSM4APIConsistency(t *testing.T) {
	// 测试基本的 SM4 加密解密功能
	key := []byte("1234123412341234")
	data := []byte("hello world")
	iv := []byte("1234123412341234")

	// 测试 CBC 模式
	encrypted, err := SM4EncryptCBCWithPKCSPadding(key, data, iv)
	if err != nil {
		t.Fatalf("SM4CBCEncrypt failed: %v", err)
	}

	decrypted, err := SM4DecryptCBCWithPKCSPadding(key, encrypted, iv)
	if err != nil {
		t.Fatalf("SM4CBCDecrypt failed: %v", err)
	}

	if string(decrypted) != string(data) {
		t.Errorf("CBC mode: decrypted data doesn't match original data")
	}

	// 测试 ECB 模式
	encrypted, err = SM4EncryptECBWithPKCSPadding(key, data, nil)
	if err != nil {
		t.Fatalf("SM4ECBEncrypt failed: %v", err)
	}

	decrypted, err = SM4DecryptECBWithPKCSPadding(key, encrypted, nil)
	if err != nil {
		t.Fatalf("SM4ECBDecrypt failed: %v", err)
	}

	if string(decrypted) != string(data) {
		t.Errorf("ECB mode: decrypted data doesn't match original data")
	}

	// 测试 CFB 模式
	encrypted, err = SM4EncryptCFBWithPKCSPadding(key, data, iv)
	if err != nil {
		t.Fatalf("SM4CFBEncrypt failed: %v", err)
	}

	decrypted, err = SM4DecryptCFBWithPKCSPadding(key, encrypted, iv)
	if err != nil {
		t.Fatalf("SM4CFBDecrypt failed: %v", err)
	}

	if string(decrypted) != string(data) {
		t.Errorf("CFB mode: decrypted data doesn't match original data")
	}

	// 测试 OFB 模式
	encrypted, err = SM4EncryptOFBWithPKCSPadding(key, data, iv)
	if err != nil {
		t.Fatalf("SM4OFBEncrypt failed: %v", err)
	}

	decrypted, err = SM4DecryptOFBWithPKCSPadding(key, encrypted, iv)
	if err != nil {
		t.Fatalf("SM4OFBDecrypt failed: %v", err)
	}

	if string(decrypted) != string(data) {
		t.Errorf("OFB mode: decrypted data doesn't match original data")
	}

	// 测试 GCM 模式
	encrypted, err = SM4GCMEncrypt(key, data, iv)
	if err != nil {
		t.Fatalf("SM4GCMEncrypt failed: %v", err)
	}

	decrypted, err = SM4GCMDecrypt(key, encrypted, iv)
	if err != nil {
		t.Fatalf("SM4GCMDecrypt failed: %v", err)
	}

	if string(decrypted) != string(data) {
		t.Errorf("GCM mode: decrypted data doesn't match original data")
	}
}

// TestSM4EncryptionModes 验证 SM4 各种模式的加密行为
// 流模式（CTR、CFB、OFB）：密文长度应等于明文长度（无 padding）
// 块模式（CBC、ECB）：密文长度应为块大小的倍数（有 padding）
func TestSM4EncryptionModes(t *testing.T) {
	key := []byte("1234567890123456") // 16 bytes key
	iv := []byte("abcdabcdabcdabcd")  // 16 bytes IV

	testCases := []struct {
		name          string
		mode          string
		plaintext     []byte
		expectPadding bool
	}{
		{"CTR/1byte", CTR, []byte("a"), false},
		{"CTR/5bytes", CTR, []byte("hello"), false},
		{"CTR/16bytes", CTR, []byte("1234567890123456"), false},
		{"CTR/17bytes", CTR, []byte("12345678901234567"), false},
		{"CTR/33bytes", CTR, []byte("123456789012345678901234567890123"), false},
		{"CFB/5bytes", CFB, []byte("hello"), false},
		{"CFB/13bytes", CFB, []byte("Hello, World!"), false},
		{"CFB/23bytes", CFB, []byte("12345678901234567890123"), false},
		{"OFB/7bytes", OFB, []byte("testing"), false},
		{"OFB/11bytes", OFB, []byte("hello world"), false},
		{"OFB/25bytes", OFB, []byte("1234567890123456789012345"), false},
		{"CBC/5bytes", CBC, []byte("hello"), true},
		{"CBC/16bytes", CBC, []byte("1234567890123456"), true},
		{"CBC/17bytes", CBC, []byte("12345678901234567"), true},
		{"ECB/5bytes", ECB, []byte("hello"), true},
		{"ECB/16bytes", ECB, []byte("1234567890123456"), true},
		{"ECB/17bytes", ECB, []byte("12345678901234567"), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ciphertext, err := SM4Enc(key, tc.plaintext, iv, tc.mode)
			require.NoError(t, err, "encryption should succeed for %s", tc.mode)
			require.NotNil(t, ciphertext, "ciphertext should not be nil for %s", tc.mode)

			if tc.expectPadding {
				require.Equal(t, 0, len(ciphertext)%16,
					"Block mode %s: ciphertext length should be multiple of block size (16 bytes)", tc.mode)
				if len(tc.plaintext)%16 != 0 {
					require.Greater(t, len(ciphertext), len(tc.plaintext),
						"Block mode %s: ciphertext should be longer than plaintext due to padding", tc.mode)
				}
			} else {
				require.Equal(t, len(tc.plaintext), len(ciphertext),
					"Stream mode %s: ciphertext length must equal plaintext length (no padding)", tc.mode)
			}

			decrypted, err := SM4Dec(key, ciphertext, iv, tc.mode)
			require.NoError(t, err, "decryption should succeed for %s", tc.mode)
			require.NotNil(t, decrypted, "decrypted data should not be nil for %s", tc.mode)

			if !tc.expectPadding {
				require.Equal(t, len(tc.plaintext), len(decrypted),
					"Stream mode %s: decrypted length should equal plaintext length", tc.mode)
				require.Equal(t, tc.plaintext, decrypted,
					"Stream mode %s: decrypted data should match original plaintext", tc.mode)
			} else {
				// 块模式：SM4Dec 不做 unpadding，解密后长度等于密文长度
				require.Equal(t, len(ciphertext), len(decrypted),
					"Block mode %s: SM4Dec doesn't unpadding, so decrypted length equals ciphertext length", tc.mode)
			}
		})
	}
}

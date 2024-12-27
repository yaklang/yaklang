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

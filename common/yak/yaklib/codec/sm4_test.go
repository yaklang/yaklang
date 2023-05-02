package codec

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
	"yaklang/common/log"
)

type encType func(key []byte, i interface{}, iv []byte) ([]byte, error)

func TestSM4(t *testing.T) {
	var count = 0
	for {
		count++
		if count > 20 {
			return
		}

		key := []byte("2345223412341234")
		target := "asdfasdfjojoq234tjoq3reaasdfasdfasdfasdfasdfjtopqd"
		for method, item := range map[string][]encType{
			"sm4_cbc": {SM4CBCEnc, SM4CBCDec},
			"sm4_cfb": {SM4CFBEnc, SM4CFBDec},
			"sm4_ecb": {SM4ECBEnc, SM4ECBDec},
			"sm4_ofb": {SM4OFBEnc, SM4OFBDec},
			"sm4_gcm": {SM4GCMEnc, SM4GCMDec},
			"aes_gcm": {AESGCMEncrypt, AESGCMDecrypt},
			"aes_cbc": {AESCBCEncrypt, AESCBCDecrypt},
		} {
			log.Infof("start to test: %v", method)
			enc := item[0]
			dec := item[1]
			tm, err := enc(key, target, nil)
			if err != nil {
				log.Error(err)
				t.FailNow()
			}
			log.Infof("enc %v finished: %v", method, StrConvQuote(string(tm)))
			origin, err := dec(key, tm, nil)
			if err != nil {
				log.Error(err)
				t.FailNow()
			}
			log.Infof("dec %v finished: %v", method, StrConvQuote(string(origin)))

			if target != string(origin) {
				log.Errorf("failed for %#v", enc)
				t.FailNow()
			}
		}
	}
}

func TestECB(t *testing.T) {
	// reM7Nv3xlWHFPhh6iomFgcrFqPg5VGegvbiyD3rKCQvuXy+kUP4satgd1c/4t1pZ23v8wXIEY14eMXPc9sXivT+jfR7iNOknzTHjZCEVBvkm0EKa2WVrIb9665ze8yRm
	data, _ := DecodeBase64("reM7Nv3xlWHFPhh6iomFgcrFqPg5VGegvbiyD3rKCQvuXy+kUP4satgd1c/4t1pZ23v8wXIEY14eMXPc9sXivT+jfR7iNOknzTHjZCEVBvkm0EKa2WVrIb9665ze8yRm")
	var result, err = SM4ECBDec([]byte("11HDESaAhiHHugDz"), data, []byte(`UISwD9fW6cFh9SNS`))
	if err != nil {
		panic(err)
	}
	if result == nil {
		panic("EMPTY RESULT")
	}
	spew.Dump(result)
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

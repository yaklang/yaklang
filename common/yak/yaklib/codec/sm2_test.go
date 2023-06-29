package codec

import (
	"testing"
)

func TestGenerateSM2PrivateKey(t *testing.T) {
	pri, pub, err := GenerateSM2PrivateKeyHEX()
	if err != nil {
		panic(err)
	}

	textOrigin := "abcasdf"
	data, err := SM2EncryptC1C2C3(pub, []byte(textOrigin))
	if err != nil {
		panic("enc c1c2c3 error")
	}

	var decrypt []byte
	count := 0
	for {
		count++
		decrypt, err = SM2DecryptC1C2C3(pri, data)
		if err != nil {
			if count > 4 {
				panic("dec c1c2c3 error")
			}
			continue
		}
		break
	}

	if string(decrypt) != textOrigin {
		panic("dec/enc failed")
	}

	textOrigin = "asdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdfasdf"
	data, err = SM2EncryptC1C3C2(pub, []byte(textOrigin))
	if err != nil {
		panic("enc c1c3c2 error")
	}

	decrypt, err = SM2DecryptC1C3C2(pri, data)
	if err != nil {
		panic("dec c1c3c2 error")
	}
	if string(decrypt) != textOrigin {
		panic("dec/enc failed")
	}

	pri, pub, err = GenerateSM2PrivateKeyPEM()
	if err != nil {
		panic(err)
	}

	textOrigin = "asdfasdfasdfasdf"
	data, err = SM2EncryptC1C2C3(pub, []byte(textOrigin))
	if err != nil {
		panic("enc c1c2c3 error")
	}

	decrypt, err = SM2DecryptC1C2C3(pri, data)
	if err != nil {
		panic("dec c1c2c3 error")
	}
	if string(decrypt) != textOrigin {
		panic("dec/enc failed")
	}

	textOrigin = "111"
	data, err = SM2EncryptC1C3C2(pub, []byte(textOrigin))
	if err != nil {
		panic("enc c1c3c2 error")
	}

	decrypt, err = SM2DecryptC1C3C2(pri, data)
	if err != nil {
		panic("dec c1c3c2 error")
	}
	if string(decrypt) != textOrigin {
		panic("dec/enc failed")
	}

	textOrigin = "111"
	data, err = SM2EncryptASN1(pub, []byte(textOrigin))
	if err != nil {
		panic("enc c1c3c2 error")
	}

	decrypt, err = SM2DecryptASN1(pri, data)
	if err != nil {
		panic("dec c1c3c2 error")
	}
	if string(decrypt) != textOrigin {
		panic("dec/enc failed")
	}

}

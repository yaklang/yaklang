package yakshell

import (
	"encoding/base64"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var EncryptMap = map[string]func(raw, key []byte) ([]byte, error){
	"": func(raw, key []byte) ([]byte, error) {
		return raw, nil
	},
	ypb.EncMode_Raw.String(): func(raw, key []byte) ([]byte, error) {
		return raw, nil
	},
	ypb.EncMode_XorBase64.String(): func(raw, key []byte) ([]byte, error) {
		return XorBase64Encode(raw, key)
	},
	ypb.EncMode_Base64.String(): func(raw, key []byte) ([]byte, error) {
		return []byte(codec.EncodeBase64(raw)), nil
	},
	//默认使用pkcs7padding
	ypb.EncMode_AesRaw.String(): func(raw, key []byte) ([]byte, error) {
		return codec.AESECBEncrypt(aesKeyPaddingWithZero(key), raw, nil)
		//return codec.AESECBEncryptWithPKCS7Padding(aesKeyPaddingWithZero(key), raw, nil)
	},
	ypb.EncMode_AesBase64.String(): func(raw, key []byte) ([]byte, error) {
		bytes, err := codec.AESECBEncrypt(aesKeyPaddingWithZero(key), raw, nil)
		if err != nil {
			return nil, err
		}
		return []byte(codec.EncodeBase64(bytes)), nil
	},
}
var DecryptMap = map[string]func(raw, key []byte) ([]byte, error){
	"": func(raw, key []byte) ([]byte, error) {
		return raw, nil
	},
	ypb.EncMode_Raw.String(): func(raw, key []byte) ([]byte, error) {
		return raw, nil
	},
	ypb.EncMode_XorBase64.String(): func(raw, key []byte) ([]byte, error) {
		return XorBase64Decode(raw, key)
	},
	ypb.EncMode_Base64.String(): func(raw, key []byte) ([]byte, error) {
		return codec.DecodeBase64(string(raw))
	},
	ypb.EncMode_AesRaw.String(): func(raw, key []byte) ([]byte, error) {
		return codec.AESECBDecrypt(raw, aesKeyPaddingWithZero(key), nil)
	},
	ypb.EncMode_AesBase64.String(): func(raw, key []byte) ([]byte, error) {
		bytes, err := codec.DecodeBase64(string(raw))
		if err != nil {
			return nil, err
		}
		return codec.AESECBDecrypt(aesKeyPaddingWithZero(key), bytes, nil)
	},
}

func XorBase64Encode(raw, key []byte) ([]byte, error) {
	for i, b := range raw {
		for _, b2 := range key {
			raw[i] = b2 ^ b
		}
	}
	return []byte(base64.StdEncoding.EncodeToString(raw)), nil
}

func XorBase64Decode(raw, key []byte) ([]byte, error) {
	var result []byte
	decodeString, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		return nil, err
	}
	for i, b := range decodeString {
		for _, b2 := range key {
			result[i] = b2 ^ b
		}
	}
	return result, nil
}

func aesKeyPaddingWithZero(key []byte) []byte {
	originLen := len(key)
	if originLen >= 32 {
		return key[:32]
	} else if originLen%16 == 0 {
		return key
	} else {
		out := make([]byte, (originLen/16+1)*16)
		copy(out, key)
		return out
	}
}

func Encryption(data, key []byte, encMode string) ([]byte, error) {
	f, exit := EncryptMap[encMode]
	if !exit {
		return nil, utils.Error("enc func not found in encode map")
	}
	return f(data, key)
}

func Decryption(data, key []byte, deMode string) ([]byte, error) {
	f, exit := DecryptMap[deMode]
	if !exit {
		return nil, utils.Error("denc func not found in decode map")
	}
	return f(data, key)
}

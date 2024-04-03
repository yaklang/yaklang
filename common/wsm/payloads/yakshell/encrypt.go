package yakshell

import (
	"encoding/base64"
	"github.com/forgoer/openssl"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type EncryptOptions func(str, key []byte) ([]byte, error)

func Base64Encode(raw []byte) ([]byte, error) {
	return []byte(base64.StdEncoding.EncodeToString(raw)), nil
}

func AesEncode(raw, key []byte) ([]byte, error) {
	return openssl.AesECBEncrypt(raw, key, openssl.PKCS5_PADDING)
}

func XorBase64Encode(raw, key []byte) ([]byte, error) {
	for i, b := range raw {
		for _, b2 := range key {
			raw[i] = b2 ^ b
		}
	}
	return []byte(base64.StdEncoding.EncodeToString(raw)), nil
}

func Base64Decode(raw []byte) ([]byte, error) {
	return base64.StdEncoding.DecodeString(string(raw))
}

func AesDecode(raw, key []byte) ([]byte, error) {
	return openssl.AesECBDecrypt(raw, key, openssl.PKCS5_PADDING)
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

func Encryption(data, key []byte, encMode string) ([]byte, error) {
	switch encMode {
	case ypb.EncMode_XorBase64.String():
		return XorBase64Encode(data, key)
	case ypb.EncMode_Base64.String():
		return Base64Encode(data)
	case ypb.EncMode_AesRaw.String():
		return AesEncode(data, key)
	default:
		return nil, utils.Errorf("encode mode not found")
	}
}

func Decryption(data, key []byte, deMode string) ([]byte, error) {
	switch deMode {
	case ypb.EncMode_XorBase64.String():
		return XorBase64Decode(data, key)
	case ypb.EncMode_Base64.String():
		return Base64Decode(data)
	case ypb.EncMode_AesRaw.String():
		return AesDecode(data, key)
	default:
		return nil, utils.Errorf("decode mode not found")
	}
}

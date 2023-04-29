package codec

import (
	"github.com/pkg/errors"
	"yaklang/common/gmsm/sm4"
)

func sm4encBase(data interface{}, key []byte, iv []byte, sm4ordinary func(key, in []byte, encode bool, iv []byte) ([]byte, error)) ([]byte, error) {
	return sm4ordinary(key, interfaceToBytes(data), true, iv)
}

func sm4decBase(data interface{}, key []byte, iv []byte, sm4ordinary func(key, in []byte, encode bool, iv []byte) ([]byte, error)) ([]byte, error) {
	return sm4ordinary(key, interfaceToBytes(data), false, iv)
}

func SM4CFBEnc(key []byte, data interface{}, iv []byte) ([]byte, error) {
	return sm4encBase(data, key, iv, sm4.Sm4CFB)
}

func SM4CBCEnc(key []byte, data interface{}, iv []byte) ([]byte, error) {
	return sm4encBase(data, key, iv, sm4.Sm4Cbc)
}

func SM4ECBEnc(key []byte, data interface{}, iv []byte) ([]byte, error) {
	return sm4encBase(data, key, iv, sm4.Sm4Ecb)
}

func SM4OFBEnc(key []byte, data interface{}, iv []byte) ([]byte, error) {
	return sm4encBase(data, key, iv, sm4.Sm4OFB)
}

func SM4CFBDec(key []byte, data interface{}, iv []byte) ([]byte, error) {
	return sm4decBase(data, key, iv, sm4.Sm4CFB)
}

func SM4CBCDec(key []byte, data interface{}, iv []byte) ([]byte, error) {
	return sm4decBase(data, key, iv, sm4.Sm4Cbc)
}

func SM4ECBDec(key []byte, data interface{}, iv []byte) ([]byte, error) {
	return sm4decBase(data, key, iv, sm4.Sm4Ecb)
}

func SM4OFBDec(key []byte, data interface{}, iv []byte) ([]byte, error) {
	return sm4decBase(data, key, iv, sm4.Sm4OFB)
}

func SM4GCMEnc(key []byte, data interface{}, iv []byte) ([]byte, error) {
	if iv == nil {
		iv = key[:sm4.BlockSize]
	}
	raw := sm4.PKCS7Padding(interfaceToBytes(data))
	result, _, err := sm4.Sm4GCM(key, iv, raw, nil, true)
	if err != nil {
		return nil, errors.Errorf("sm4 gcm enc failed: %s", err)
	}
	return result, nil
}

func SM4GCMDec(key []byte, data interface{}, iv []byte) ([]byte, error) {
	if iv == nil {
		iv = key[:sm4.BlockSize]
	}

	result, _, err := sm4.Sm4GCM(key, iv, interfaceToBytes(data), nil, false)
	if err != nil {
		return nil, errors.Errorf("sm4 gcm dec failed: %s", err)
	}
	return sm4.PKCS7UnPadding(result), nil
}

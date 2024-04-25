package codec

import (
	"bytes"
	"github.com/yaklang/yaklang/common/gmsm/sm4"
)

var PKCS7Padding = sm4.PKCS7Padding
var PKCS7UnPadding = sm4.PKCS7UnPadding

func PKCS7PaddingFor8ByteBlock(src []byte) []byte {
	return PKCS5Padding(src, 8)
}

func PKCS7UnPaddingFor8ByteBlock(src []byte) []byte {
	return PKCS5UnPadding(src)
}

func AesKeyPaddingWithZero(key []byte) []byte {
	k := len(key)
	count := 0
	switch true {
	case k <= 16:
		count = 16 - k
	case k <= 24:
		count = 24 - k
	case k <= 32:
		count = 32 - k
	default:
		return key[:32]
	}
	if count == 0 {
		return key
	}

	return append(key, bytes.Repeat([]byte{0}, count)...)
}

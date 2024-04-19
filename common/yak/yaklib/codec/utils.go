package codec

import (
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

type EncodedFunc func(any) string

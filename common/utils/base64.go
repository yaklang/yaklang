package utils

import "github.com/yaklang/yaklang/common/yak/yaklib/codec"

func IsBase64(s string) bool {
	var ret []byte
	var err error
	s = codec.ForceQueryUnescape(s)
	ret, err = codec.DecodeBase64(s)
	if err != nil {
		return false
	}

	if codec.IsUtf8(ret) {
		//isSafe := true
		//for _, b := range ret {
		//	if b >= utf8.RuneSelf {
		//		isSafe = false
		//		break
		//	}
		//}
		//if isSafe {
		//	return true
		//}
		return true
	}

	if _, err = GzipDeCompress(ret); err == nil {
		// gzip
		return true
	} else if _, err = ZlibDeCompress(ret); err == nil {
		// zlib
		return true
	} else if _, err := codec.GB18030ToUtf8(ret); err == nil {
		return true
	}
	return false
}

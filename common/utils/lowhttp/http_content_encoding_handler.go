package lowhttp

import (
	"github.com/yaklang/yaklang/common/log"
)

func ContentEncodingDecode(contentEncoding string, bodyRaw []byte) (finalResult []byte, fixed bool) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("handle content-encoding decode failed! reason: %s", err)
			finalResult = bodyRaw
			fixed = false
		}
	}()

	result, _, ok := _decodeByHeaderOrMagic(contentEncoding, bodyRaw, true, _autoUnzipMaxDecodedBodyBytes)
	return result, ok
}

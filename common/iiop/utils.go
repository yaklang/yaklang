package iiop

import "github.com/yaklang/yaklang/common/yserx"

func paddingStr(old []byte) []byte {

	l := len(old)
	res := yserx.IntTo4Bytes(l)
	res = append(res, old...)
	if (l % 4) != 0 {
		excpectL := ((l / 4) + 1) * 4
		for i := 0; i < excpectL-l; i++ {
			res = append(res, 0x00)
		}
	}
	return res
}

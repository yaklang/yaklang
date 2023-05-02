package iiop

import (
	"testing"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
)

func TestParseIiop(t *testing.T) {
	req := GetRebindMsgTmp()
	println(codec.EncodeToHex(req.Bytes()))
}

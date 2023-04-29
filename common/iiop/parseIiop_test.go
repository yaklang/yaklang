package iiop

import (
	"yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestParseIiop(t *testing.T) {
	req := GetRebindMsgTmp()
	println(codec.EncodeToHex(req.Bytes()))
}

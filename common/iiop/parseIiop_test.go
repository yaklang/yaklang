package iiop

import (
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestParseIiop(t *testing.T) {
	req := GetRebindMsgTmp()
	println(codec.EncodeToHex(req.Bytes()))
}

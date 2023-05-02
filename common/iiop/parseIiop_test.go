package iiop

import (
	"testing"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func TestParseIiop(t *testing.T) {
	req := GetRebindMsgTmp()
	println(codec.EncodeToHex(req.Bytes()))
}

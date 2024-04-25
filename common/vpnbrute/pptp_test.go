package vpnbrute

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/vpnbrute/ppp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestAAABBB(t *testing.T) {
	err, ok := Auth(context.Background(), "172.22.175.238:1723", "test", "123456")
	_ = ok
	if ok {
		println("ok")
	}
	if err != nil {
		return
	}
}

func TestAab(t *testing.T) {
	a := ipChecksum(0x2f, 0x2f, []byte{0xac, 0x1b, 0xa0, 0x01}, []byte{0xac, 0x1b, 0xa7, 0xce})
	println(a)
}

func TestChap_M5(t *testing.T) {
	challenge, _ := codec.DecodeHex("01d000bb3535999a8e1fa218901fe4001b5f63")
	fmt.Println(codec.EncodeToHex(ppp.GenerateCHAPMD5Response([]byte{0xe2}, []byte("123456"), challenge)))
}

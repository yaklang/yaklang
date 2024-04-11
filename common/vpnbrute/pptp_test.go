package vpnbrute

import (
	"github.com/yaklang/yaklang/common/vpnbrute/ppp"
	"testing"
)

func TestAAABBB(t *testing.T) {
	a := GetDefaultPPTPAuth()
	a.Target = "172.28.20.215:1723"
	a.ppp = ppp.GetDefaultPPPAuth()
	a.ppp.Username = "test"
	a.ppp.Password = "1234"
	a.ppp.AuthTypeCode = ppp.CHAP_MD5
	err, ok := a.Auth()
	_ = ok
	if err != nil {
		return
	}
}

func TestAab(t *testing.T) {
	a := ipChecksum(0x2f, 0x2f, []byte{0xac, 0x1b, 0xa0, 0x01}, []byte{0xac, 0x1b, 0xa7, 0xce})
	println(a)
}

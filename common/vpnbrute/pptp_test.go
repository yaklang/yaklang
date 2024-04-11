package vpnbrute

import (
	"github.com/yaklang/yaklang/common/vpnbrute/ppp"
	"testing"
)

func TestAAABBB(t *testing.T) {
	a := &PPTPAuth{
		Target: "192.168.212.208:1723",
	}
	a.ppp = ppp.GetDefaultPPPAuth()
	a.ppp.Username = "test"
	a.ppp.Password = "123456"
	a.ppp.AuthTypeCode = ppp.CHAP_MD5
	a.Auth()
}

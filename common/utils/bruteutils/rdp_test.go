package bruteutils

import (
	"yaklang/common/log"
	"testing"

	"github.com/icodeface/grdp"
	"github.com/icodeface/grdp/glog"
)

func TestRdpClient_Login(t *testing.T) {
	r, err := rdpLogin("127.0.0.1", "DESKTOP-Q1Test", "administrator", "12345116", 3389)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	_ = r
}

func TestBruteItem_Result(t *testing.T) {
	client := grdp.NewClient("127.0.0.1:3389", glog.DEBUG)
	err := client.Login("administrator", "123456")
	if err != nil {
		log.Error(err)
	}
}

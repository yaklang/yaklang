package bruteutils

import (
	"github.com/yaklang/yaklang/common/log"
	"testing"
)

func TestRdpClient_Login(t *testing.T) {
	t.SkipNow()

	r, err := rdpLogin("127.0.0.1", "DESKTOP-Q1Test", "administrator", "12345116", 3389)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	_ = r
}

func TestBruteItem_Result(t *testing.T) {
	t.SkipNow()
}

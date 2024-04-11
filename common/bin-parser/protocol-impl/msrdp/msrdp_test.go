package msrdp

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
	"testing"
)

func TestMsrdp(t *testing.T) {
	_, err := Login("129.153.219.71", 3389, "123456", "Administrator", "")
	if err != nil {
		t.Fatal(err)
	} else {
		println("login successful")
	}
}
func TestGrdp(t *testing.T) {
	host := "129.153.219.71"
	port := 3389
	client := grdp.NewClient(utils.HostPort(host, port), glog.DEBUG)
	err := client.Login("", "Administrator", "123456")
	if err != nil {
		t.Fatal(err)
	}

}

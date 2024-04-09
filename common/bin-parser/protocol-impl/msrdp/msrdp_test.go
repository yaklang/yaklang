package msrdp

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
	"testing"
)

func TestMsrdp(t *testing.T) {
	_, err := Login("47.120.44.219", 3389, "", "Administrator", "xL47r3@bw9.g7E8")
	if err != nil {
		t.Fatal(err)
	}
}
func TestGrdp(t *testing.T) {
	host := "47.120.44.219"
	port := 3389
	client := grdp.NewClient(utils.HostPort(host, port), glog.DEBUG)
	err := client.Login("", "Administrator", "xL47r3@bw9.g7E8")
	if err != nil {
		t.Fatal(err)
	}

}

package msrdp

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
	"testing"
)

func TestMsrdp(t *testing.T) {
	client, err := NewRDPClient("47.120.44.219:3389")
	if err != nil {
		t.Fatal(err)
	}
	err = client.Login("", "Administrator", "g.cXgKg.hjh1RY]*R1>s")
	if err != nil {
		t.Fatal(err)
	} else {
		println("login successful")
	}
}
func TestGrdp(t *testing.T) {
	host := "47.120.44.219"
	port := 3389
	client := grdp.NewClient(utils.HostPort(host, port), glog.DEBUG)
	err := client.Login("", "Administrator", "g.cXgKg.hjh1RY]*R1>s")
	if err != nil {
		t.Fatal(err)
	}
}

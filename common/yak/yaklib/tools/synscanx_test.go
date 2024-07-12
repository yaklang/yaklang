package tools

import (
	"github.com/yaklang/yaklang/common/synscanx"
	"testing"
)

func Test__scanx(t *testing.T) {
	res, err := _scanx(
		"192.168.3.1-255",
		//"47.52.100.35/24",
		"80",
		synscanx.WithIface("WLAN"),
	)
	if err != nil {
		t.Fatal(err)

	}
	for re := range res {
		t.Log(re.String())
	}
}

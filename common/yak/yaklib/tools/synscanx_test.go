package tools

import (
	"github.com/yaklang/yaklang/common/synscanx"
	_ "net/http/pprof"
	"testing"
)

func Test__scanx(t *testing.T) {
	res, err := _scanx(
		"192.168.124.1/24",
		//"47.52.100.35/24",
		"21,22,23,80,443",
		synscanx.WithIface("WLAN 4"),
	)
	if err != nil {
		t.Fatal(err)
	}
	for re := range res {
		t.Log(re.String())
	}
}

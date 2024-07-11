package tools

import (
	"github.com/yaklang/yaklang/common/synscanx"
	"testing"
)

func Test__scanx(t *testing.T) {
	res, err := _scanx(
		"192.168.3.2,8.8.8.8",
		"21,22,23,80,443",
		synscanx.WithIface("WLAN"),
	)
	if err != nil {
		t.Fatal(err)

	}
	for re := range res {
		t.Log(re.String())
	}
}

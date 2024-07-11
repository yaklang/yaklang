package tools

import (
	"github.com/yaklang/yaklang/common/synscanx"
	"testing"
)

func Test__scanx(t *testing.T) {
	res, err := _scanx(
		"47.52.100.35/24",
		"21",
		synscanx.WithIface("WLAN 4"),
	)
	if err != nil {
		t.Fatal(err)

	}
	for re := range res {
		t.Log(re.String())
	}
}

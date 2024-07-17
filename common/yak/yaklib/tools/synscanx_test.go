package tools

import (
	"github.com/yaklang/yaklang/common/synscanx"
	_ "net/http/pprof"
	"testing"
)

func Test__scanx(t *testing.T) {
	synPacketCounter := 0
	addSynPacketCounter := func() {
		synPacketCounter++
	}

	res, err := _scanx(
		"192.168.3.0,192.168.3.1,192.168.3.2,192.168.3.3,192.168.3.4,192.168.3.5,192.168.3.6,192.168.3.7,192.168.3.8,192.168.3.9,192.168.3.10,192.168.3.11,192.168.3.12,192.168.3.13,192.168.3.14,192.168.3.15,192.168.3.16,192.168.3.17,192.168.3.18,192.168.3.19",
		//"47.52.100.35/24",
		"21,22,23,80,443,3306",
		synscanx.WithSubmitTaskCallback(func(i string) {
			addSynPacketCounter()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	for re := range res {
		t.Log(re.String())
	}
	t.Log("synPacketCounter:", synPacketCounter)
}

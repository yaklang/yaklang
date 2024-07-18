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
		"192.168.3.3/24",
		//"47.52.100.35/24",
		"21,22,23,80,443,3306",
		//synscanx.WithInitFilterPorts("443"),
		synscanx.WithConcurrent(1000),
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

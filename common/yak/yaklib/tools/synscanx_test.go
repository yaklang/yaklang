package tools

import (
	"github.com/yaklang/yaklang/common/synscanx"
	_ "net/http/pprof"
	"testing"
)

func Test__scanx(t *testing.T) {
	//nmapRuleConfig := fp.NewConfig(
	//	fp.WithActiveMode(true),
	//	fp.WithTransportProtos(fp.UDP),
	//	fp.WithProbesMax(3),
	//)
	//firstBlock, blocks, bestMode := fp.GetRuleBlockByConfig(53, nmapRuleConfig)
	//if bestMode {
	//}
	//t.Log(firstBlock, blocks, bestMode)
	synPacketCounter := 0
	addSynPacketCounter := func() {
		synPacketCounter++
	}

	res, err := _scanx(
		"192.168.124.51/24",
		//"47.52.100.35/24",
		//"U:137",
		"21,22,23,80,443",
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

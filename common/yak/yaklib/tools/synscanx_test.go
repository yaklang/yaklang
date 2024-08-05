package tools

import (
	"github.com/yaklang/yaklang/common/synscanx"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	_ "net/http/pprof"
	"testing"
)

func Test__scanx(t *testing.T) {
	synPacketCounter := 0
	addSynPacketCounter := func() {
		synPacketCounter++
	}

	res, err := _scanx(
		//"192.168.3.2/24",
		//"47.52.100.35/24",
		"baidu.com",
		//"U:137",
		"22,21,80,443",
		//synscanx.WithInitFilterPorts("443"),
		synscanx.WithWaiting(5),
		synscanx.WithConcurrent(2000),
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

func Test___scanxFromPingUtils(t *testing.T) {
	synPacketCounter := 0
	addSynPacketCounter := func() {
		synPacketCounter++
	}
	list := []string{
		"47.52.100.35",
		"47.52.100.228",
		"47.52.100.30",
		"47.52.100.24",
		"47.52.100.33",
		"47.52.100.123",
		"47.52.100.75",
	}

	c := make(chan *pingutil.PingResult)
	go func() {
		defer close(c)
		for _, ip := range list {
			c <- &pingutil.PingResult{
				IP: ip,
				Ok: true,
			}
		}
	}()

	res, err := _scanxFromPingUtils(
		c,
		//"47.52.100.35/24",
		//"U:137",
		"22,21,80,443",
		//synscanx.WithInitFilterPorts("443"),
		synscanx.WithWaiting(5),
		synscanx.WithConcurrent(2000),
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

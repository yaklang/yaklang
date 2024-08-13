package tools

import (
	"github.com/yaklang/yaklang/common/synscanx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	_ "net/http/pprof"
	"sync"
	"testing"
	"time"
)

func Test__scanx(t *testing.T) {
	synPacketCounter := 0
	addSynPacketCounter := func() {
		synPacketCounter++
	}

	startSYNPacketCounter := func() {
		go func() {
			for {
				time.Sleep(2 * time.Second)
				t.Log("SYN 发包数", synPacketCounter)
			}
		}()
	}
	startSYNPacketCounter()

	res, err := _scanx(
		"192.168.3.50",
		//"47.52.100.35/24",
		//"baidu.com",
		//"U:137",
		"22,21,80,443",
		//synscanx.WithInitFilterPorts("443"),
		synscanx.WithWaiting(5),
		synscanx.WithShuffle(false),
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
		"192.168.3.50",
		//"192.168.3.116",
		//"192.168.3.122",
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

func Test___filter(t *testing.T) {

	wg := sync.WaitGroup{}
	for {
		filter := utils.NewPortsFilter()

		wg.Add(2)
		go func() {
			defer wg.Done()
			filter.Add("1-100")
			filter = nil
		}()
		time.Sleep(100 * time.Millisecond)

		go func() {
			defer wg.Done()

			if filter.Contains(1) {
				t.Log("contains 1")
			}
		}()

		wg.Wait()
	}

}

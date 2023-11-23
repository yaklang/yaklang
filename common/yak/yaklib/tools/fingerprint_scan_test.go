package tools

import (
	"fmt"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/synscan"
	"sync"
	"testing"
	"time"
)

func Test_scanFingerprint(t *testing.T) {

	target := "127.0.0.1"

	port := "55072"

	protoList := []interface{}{"tcp", "udp"}

	pp := func(proto ...interface{}) fp.ConfigOption {
		return fp.WithTransportProtos(fp.ParseStringToProto(proto...)...)
	}

	ch, err := scanFingerprint(target, port, pp(protoList...),
		fp.WithProbeTimeoutHumanRead(5),
		fp.WithProbesMax(100),
	)
	//ch, err := scanFingerprint(target, "162", pp(protoList...), fp.WithProbeTimeoutHumanRead(5))

	if err != nil {
		t.Error(err)
	}

	for v := range ch {
		fmt.Println(v.String())
	}
}

func Test_scanFingerprint1(t *testing.T) {
	target := "192.168.3.104"

	tcpPorts := "3306,9090"
	synPorts := "6379,9090"

	tcpScan := func(addr string) {
		ch, err := scanFingerprint(
			addr, tcpPorts,
		)

		if err != nil {
			t.FailNow()
		}

		for v := range ch {
			fmt.Println("TCPGOT " + v.String())
		}
	}

	Scan := func(target string, port string, opts ...scanOpt) (chan *synscan.SynScanResult, error) {
		config := &_yakPortScanConfig{
			waiting:           5 * time.Second,
			rateLimitDelayMs:  1,
			rateLimitDelayGap: 5,
		}
		for _, opt := range opts {
			opt(config)
		}
		return _synScanDo(hostsToChan(target), port, config)
	}

	synScan := func(addr string) {
		res, err := Scan(target, synPorts, _scanOptExcludePorts(tcpPorts))
		//res, err := Scan(target, synPorts, _scanOptOpenPortInitPortFilter("6379"))
		//res, err := Scan(target, synPorts)
		if err != nil {
			t.FailNow()
		}
		res2, err := _scanFromTargetStream(res)
		if err != nil {
			t.FailNow()
		}
		for result := range res2 {
			fmt.Println("SYNGOT " + result.String())
		}
	}
	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		synScan(target)
	}()

	go func() {
		defer wg.Done()
		tcpScan(target)
	}()

	wg.Wait()
}

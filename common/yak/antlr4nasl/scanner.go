package antlr4nasl

import (
	"context"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"os"
)

func SynScan(hosts string, ports string) ([]int, error) {
	openPorts := []int{}
	os.Setenv("YAKMODE", "vm")
	yak.Init()
	yakEngine := yaklang.New()

	yakEngine.SetVar("addRes", func(n int) {
		openPorts = append(openPorts, n)
	})

	yakEngine.SetVar("hosts", hosts)
	yakEngine.SetVar("ports", ports)

	err := yakEngine.SafeEval(context.Background(), `

getPingScan = func() {
	return ping.Scan(hosts,ping.timeout(5), ping.concurrent(10)) 
}

res, err := servicescan.ScanFromPing(
	getPingScan(), 
	ports)
die(err)

for result = range res {
	if result.IsOpen(){
		addRes(result.Port)	
	}
}

`)
	if err != nil {
		return nil, err
	}
	return openPorts, nil
}

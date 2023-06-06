package antlr4nasl

import (
	"context"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"os"
)

// 临时的，用于测试
func ServiceScan(hosts string, ports string, proxies ...string) ([]*fp.MatchResult, error) {
	result := []*fp.MatchResult{}
	os.Setenv("YAKMODE", "vm")
	yakEngine := yaklang.New()

	yakEngine.SetVar("addRes", func(res *fp.MatchResult) {
		result = append(result, res)
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
		addRes(result)	
	}
}

`)
	if err != nil {
		return nil, err
	}
	return result, nil
}

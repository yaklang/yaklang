package aibp

import (
	"testing"

	"github.com/yaklang/yaklang/common/aiforge"
)

func TestGenForgeMetadata(t *testing.T) {
	res, err := aiforge.GenerateForgeMetadata(`
host = cli.String("host")
ports = cli.StringSlice("ports")
resCh = synscan.SynScan(host, ports)
for res := range resCh {
	yakit.log(res.String())
}
`)
	if err != nil {
		t.Fatal(err)
	}
	println(res.Description)
}

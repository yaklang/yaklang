package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestGenerateMetadata(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	res, err := plugins_rag.GenerateYakScriptMetadata(`
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

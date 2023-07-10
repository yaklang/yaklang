package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func TestGRPCMUSTPASS_SMOKING_EVALUATE_PLUGIN(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	name, err := yakit.CreateTemporaryYakScript("port-scan", `yakit.AutoInitYakit()
handle = result => {
	yakit.Info("HELLO")
	risk.NewRisk("http://baidu.com")
}`)
	if err != nil {
		panic(err)
	}
	rsp, err := client.SmokingEvaluatePlugin(context.Background(), &ypb.SmokingEvaluatePluginRequest{
		PluginName: name,
	})
	if err != nil {
		panic(err)
	}
	var checking = false
	for _, r := range rsp.Results {
		spew.Dump(r)
		if strings.Contains(r.String(), "[Negative Alarm]") {
			checking = true
		}
	}
	if !checking {
		panic("should have negative alarm")
	}

	name, err = yakit.CreateTemporaryYakScript("port-scan", `yakit.AutoInitYakit()
handle = result => {
	yakit.Info(bacd)
	risk.NewRisk("http://baidu.com")
}`)
	if err != nil {
		panic(err)
	}
	rsp, err = client.SmokingEvaluatePlugin(context.Background(), &ypb.SmokingEvaluatePluginRequest{
		PluginName: name,
	})
	if err != nil {
		panic(err)
	}
	spew.Dump(rsp)
	checking = false
	for _, r := range rsp.Results {
		spew.Dump(r)
		if strings.Contains(r.String(), "undefined variable") {
			checking = true
		}
	}
	if !checking {
		panic("should have negative alarm")
	}
}

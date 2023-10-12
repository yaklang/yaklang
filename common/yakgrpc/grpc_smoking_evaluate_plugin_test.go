package yakgrpc

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SMOKING_EVALUATE_PLUGIN(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	name, err := yakit.CreateTemporaryYakScript("port-scan", `yakit.AutoInitYakit()
handle = result => {
	yakit.Info("HELLO")
	risk.NewRisk("http://baidu.com")
}`)
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := client.SmokingEvaluatePlugin(context.Background(), &ypb.SmokingEvaluatePluginRequest{
		PluginName: name,
	})
	if err != nil {
		t.Fatal(err)
	}
	var checking = false
	for _, r := range rsp.Results {
		// spew.Dump(r)
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
		t.Fatal(err)
	}
	rsp, err = client.SmokingEvaluatePlugin(context.Background(), &ypb.SmokingEvaluatePluginRequest{
		PluginName: name,
	})
	if err != nil {
		t.Fatal(err)
	}
	// spew.Dump(rsp)
	checking = false
	for _, r := range rsp.Results {
		// spew.Dump(r)
		if strings.Contains(r.String(), "undefined variable") {
			checking = true
		}
	}
	if !checking {
		t.Fatal("should have negative alarm")
	}
}

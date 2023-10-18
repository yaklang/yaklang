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

	type testCase struct {
		code      string
		err       string
		zeroScore bool // is score == 0 ?
	}
	TestSmokingEvaluatePlugin := func(tc testCase) {
		name, err := yakit.CreateTemporaryYakScript("port-scan", tc.code)
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
		// spew.Dump("result: ", rsp)
		if tc.zeroScore && rsp.Score != 0 {
			// want score == 0 but get !0
			t.Fatal("this test should have score = 0")
		}
		if !tc.zeroScore && rsp.Score == 0 {
			// want score != 0 but get 0
			t.Fatal("this test shouldn't have score = 0")
		}
		for _, r := range rsp.Results {
			if strings.Contains(r.String(), tc.err) {
				checking = true
			}
		}
		if !checking {
			t.Fatalf("should have %s", tc.err)
		}
	}

	t.Run("test negative alarm", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
handle = result => {
	yakit.Info("HELLO")
	risk.NewRisk("http://baidu.com")
}`,
			err:       "[Negative Alarm]",
			zeroScore: false,
		})
	})

	t.Run("test undefine variable", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
handle = result => {
	yakit.Info(bacd)
	risk.NewRisk("http://baidu.com")
}`,
			err:       "Value undefine",
			zeroScore: true,
		})
	})

	t.Run("test just warning", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
handle = result => {
}`,
			err:       "empty block",
			zeroScore: false,
		})
	})
}

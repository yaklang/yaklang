package yakgrpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_LANGUAGE_SMOKING_EVALUATE_PLUGIN_BATCH(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	type code struct {
		src string
		typ string
	}

	test := func(codes []code) {
		names := make([]string, 0, len(codes))
		for _, c := range codes {
			typ := c.typ
			if typ == "" {
				typ = "port-scan"
			}
			name, err := yakit.CreateTemporaryYakScript(typ, c.src)
			if err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}

		fmt.Println("names:", names)
		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: names,
		})
		if err != nil {
			t.Fatal(err)
		}
		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}
			t.Log(res)
		}
	}

	t.Run("test base ", func(t *testing.T) {
		a := (0 + 1) / 4
		fmt.Println(a)
		test([]code{
			{
				src: `
yakit.AutoInitYakit()
handle = result => {
	yakit.Info("HELLO")
	// risk.NewRisk("http://baidu.com")
}
			`,
				typ: ``,
			},
			{
				src: `
			print(aa) // undefine
			`,
				typ: ``,
			},
			{
				src: `
			print(aa) // undefine
			`,
				typ: ``,
			},
			{
				src: `
			print(aa) // undefine
			`,
				typ: ``,
			},
		})
	})
}

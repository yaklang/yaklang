package coreplugin

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"testing"
)

func TestGRPCMUSTPASS_SSTI(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		panic(err)
	}

	vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}

	pluginName := "SSTI Expr 服务器模版表达式注入"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}
	vul := VulInfo{
		Path: []string{"/expr/injection?a=1", "/expr/injection?b={%22a%22:%201}", "/expr/injection?c=abc"},
		ExpectedResult: map[string]int{
			fmt.Sprintf("SSTI Expr Injection (Param:a): %s/expr/injection?a=", vulAddr): 3,
			fmt.Sprintf("SSTI Expr Injection (Param:b): %s/expr/injection?b=", vulAddr): 3,
			fmt.Sprintf("SSTI Expr Injection (Param:c): %s/expr/injection?c=", vulAddr): 3,
		},
		StrictMode: false,
	}

	Must(TestCoreMitmPlug(pluginName, server, vul, client, t), "SSTI插件对于注入检测结果不符合预期")
}

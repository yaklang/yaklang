package coreplugin

import (
	"context"
	"github.com/yaklang/yaklang/common/vulinbox"
	"testing"
)

func TestGRPCMUSTPASS_SSTI(t *testing.T) {
	client, err := NewLocalClient()
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
	vul1 := VulInfo{
		Path:           "/expr/injection?a=1",
		ExpectedResult: map[string]int{"表达式注入成功检测：参数：Name:a": 1},
	}
	vul2 := VulInfo{
		Path:           "/expr/injection?b={%22a%22:%201}",
		ExpectedResult: map[string]int{"表达式注入成功检测：参数：Name:b": 1},
	}
	vul3 := VulInfo{
		Path:           "/expr/injection?c=abc",
		ExpectedResult: map[string]int{"表达式注入成功检测：参数：Name:c": 1},
	}

	Must(TestCoreMitmPlug(pluginName, server, vul1, client, t), "SSTI插件对于?a注入检测结果不符合预期")
	Must(TestCoreMitmPlug(pluginName, server, vul2, client, t), "SSTI插件对于?b注入检测结果不符合预期")
	Must(TestCoreMitmPlug(pluginName, server, vul3, client, t), "SSTI插件对于?c注入检测结果不符合预期c")
}

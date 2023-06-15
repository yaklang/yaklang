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

	plug := PlugInfo{
		PlugName:    "Basic SSTI检测插件",
		BinDataPath: "data/base-yak-plugin/Basic SSTI.yak",
	}
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}
	vul1 := VulInfo{
		Path:           "/expr/injection?a=1",
		ExpectedResult: map[string]int{"表达式注入成功检测：参数：Name:a": 3},
	}
	vul2 := VulInfo{
		Path:           "/expr/injection?b={%22a%22:%201}",
		ExpectedResult: map[string]int{"表达式注入成功检测：参数：Name:b": 3},
	}
	vul3 := VulInfo{
		Path:           "/expr/injection?c=abc",
		ExpectedResult: map[string]int{"表达式注入成功检测：参数：Name:c": 3},
	}

	Must(TestMitmPlug(plug, server, vul1, client, t), "SSTI插件对于?a注入检测结果不符合预期")
	Must(TestMitmPlug(plug, server, vul2, client, t), "SSTI插件对于?b注入检测结果不符合预期")
	Must(TestMitmPlug(plug, server, vul3, client, t), "SSTI插件对于?c注入检测结果不符合预期c")
}

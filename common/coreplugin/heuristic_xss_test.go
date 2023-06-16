package coreplugin

import (
	"context"
	"github.com/yaklang/yaklang/common/vulinbox"
	"testing"
)

func TestGRPCMUSTPASS_XSS(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}

	pluginName := "基础 XSS 检测"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}
	vul1 := VulInfo{
		Path:           "/xss/safe?name=admin",
		ExpectedResult: map[string]int{"": 1},
	}
	vul2 := VulInfo{
		Path:           "/xss/echo?name=admin",
		ExpectedResult: map[string]int{"": 1},
	}
	vul3 := VulInfo{
		Path:           "/xss/replace/nocase?name=admin",
		ExpectedResult: map[string]int{"": 1},
	}

	Must(TestCoreMitmPlug(pluginName, server, vul1, client, t), " ")
	Must(TestCoreMitmPlug(pluginName, server, vul2, client, t), " ")
	Must(TestCoreMitmPlug(pluginName, server, vul3, client, t), " ")
}

package coreplugin

import (
	"context"
	"github.com/yaklang/yaklang/common/vulinbox"
	"testing"
)

func TestGRPCMUSTPASS_Shiro(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}

	pluginName := "Shiro 指纹识别 + 弱密码检测"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}
	vul1 := VulInfo{
		Path:           "/shiro/cbc",
		ExpectedResult: map[string]int{"检测到 Shiro(Cookie) 框架使用": 1, "(Shiro 远程代码执行)": 1},
	}
	vul2 := VulInfo{
		Path:           "/shiro/gcm",
		ExpectedResult: map[string]int{"检测到 Shiro(Cookie) 框架使用": 1, "(Shiro 远程代码执行)": 1},
	}

	Must(TestCoreMitmPlug(pluginName, server, vul1, client, t), "Shiro插件对于低版本shiro检测结果不符合预期")
	Must(TestCoreMitmPlug(pluginName, server, vul2, client, t), "Shiro插件对于高版本shiro检测结果不符合预期")
}

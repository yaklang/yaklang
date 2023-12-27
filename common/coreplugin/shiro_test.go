package coreplugin

import (
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc"
)

func TestGRPCMUSTPASS_Shiro(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		panic(err)
	}
	pluginName := "Shiro 指纹识别 + 弱密码检测"
	vul1 := VulInfo{
		Path: []string{"/shiro/cbc"},
		ExpectedResult: map[string]int{
			"检测到 Shiro(Cookie) 框架使用": 1,
			"(Shiro 默认 KEY)":         1,
			"(Shiro Header 回显)":      1,
		},
		StrictMode: false,
	}
	vul2 := VulInfo{
		Path: []string{"/shiro/gcm"},
		ExpectedResult: map[string]int{
			"检测到 Shiro(Cookie) 框架使用": 1,
			"(Shiro 默认 KEY)":         1,
			"(Shiro Header 回显)":      1,
		},
		StrictMode: false,
	}

	Must(CoreMitmPlugTest(pluginName, server, vul1, client, t), "Shiro插件对于低版本shiro检测结果不符合预期")
	Must(CoreMitmPlugTest(pluginName, server, vul2, client, t), "Shiro插件对于高版本shiro检测结果不符合预期")
}

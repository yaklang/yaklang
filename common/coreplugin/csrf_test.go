package coreplugin

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/vulinbox"
	"testing"
)

func TestGRPCMUSTPASS_CSRF(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}

	pluginName := "CSRF 表单保护与 CORS 配置不当检测"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}

	vul := VulInfo{
		Path: []string{
			"/csrf/unsafe",
			"/csrf/safe",
		},
		ExpectedResult: map[string]int{
			fmt.Sprintf("XSS for: %s/xss/echo?name=admin", vulAddr):           1,
			fmt.Sprintf("XSS for: %s/xss/replace/nocase?name=admin", vulAddr): 1,
		},
		StrictMode: true,
	}

	Must(TestCoreMitmPlug(pluginName, server, vul, client, t), " ")

}

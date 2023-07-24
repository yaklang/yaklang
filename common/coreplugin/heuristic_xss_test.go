package coreplugin

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/vulinbox"
	"testing"
)

func TestGRPCMUSTPASS_XSS(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	_, vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}

	pluginName := "基础 XSS 检测"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}

	vul := VulInfo{
		Path: []string{
			"/xss/safe?name=admin",
			"/xss/echo?name=admin",
			"/xss/replace/nocase?name=admin",
		},
		ExpectedResult: map[string]int{
			fmt.Sprintf("XSS for: %s/xss/echo?name=admin", vulAddr):           1,
			fmt.Sprintf("XSS for: %s/xss/replace/nocase?name=admin", vulAddr): 1,
		},
		StrictMode: true,
	}

	Must(TestCoreMitmPlug(pluginName, server, vul, client, t), " ")

}

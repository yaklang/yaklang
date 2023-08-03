package coreplugin

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"testing"
)

func TestGRPCMUSTPASS_CSRF(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
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
			fmt.Sprintf("csrf for: %v/csrf/unsafe", vulAddr): 1,
			fmt.Sprintf("csrf for: %v/csrf/safe", vulAddr):   0,
		},
		StrictMode: true,
	}

	Must(TestCoreMitmPlug(pluginName, server, vul, client, t), " ")

}

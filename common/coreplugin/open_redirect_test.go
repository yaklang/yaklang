package coreplugin

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"testing"
)

func TestGRPCMUSTPASS_OPEN_REDIRECT(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		panic(err)
	}

	vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}

	pluginName := "开放 URL 重定向漏洞"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}

	vul := VulInfo{
		Path: []string{
			"/ssrf/redirect/safe?redirect=/redirect/main",
			"/ssrf/redirect/meta/case2?redirect=/redirect/main",
			"/ssrf/redirect/meta/case1?redirect=/redirect/main",
			"/ssrf/redirect/js/basic2?redirect=/redirect/main",
			"/ssrf/redirect/js/basic1?redirect_to=/redirect/main",
			"/ssrf/redirect/js/basic?redUrl=/redirect/main",
			"/ssrf/redirect/redirect-hell?destUrl=/redirect/main",
			"/ssrf/redirect/basic?destUrl=/redirect/main",
		},
		ExpectedResult: map[string]int{
			fmt.Sprintf("URL redirect for: %v/ssrf/redirect/safe", vulAddr):          0,
			fmt.Sprintf("URL redirect for: %v/ssrf/redirect/meta/case2", vulAddr):    1,
			fmt.Sprintf("URL redirect for: %v/ssrf/redirect/meta/case1", vulAddr):    1,
			fmt.Sprintf("URL redirect for: %v/ssrf/redirect/js/basic2", vulAddr):     1,
			fmt.Sprintf("URL redirect for: %v/ssrf/redirect/js/basic1", vulAddr):     1,
			fmt.Sprintf("URL redirect for: %v/ssrf/redirect/js/basic?", vulAddr):     1,
			fmt.Sprintf("URL redirect for: %v/ssrf/redirect/redirect-hell", vulAddr): 1,
			fmt.Sprintf("URL redirect for: %v/ssrf/redirect/basic", vulAddr):         1,
		},
		StrictMode: false,
	}

	Must(TestCoreMitmPlug(pluginName, server, vul, client, t), " ")

}

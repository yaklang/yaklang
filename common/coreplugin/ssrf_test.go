package coreplugin

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SSRF(t *testing.T) {
	client, err := yakgrpc.NewLocalClient(true)
	if err != nil {
		panic(err)
	}

	pluginName := "SSRF HTTP Public"
	vulGet := VulInfo{
		Path: []string{
			"/ssrf/in-get?url=",
			"/ssrf/safe?url=",
		},
		ExpectedResult: map[string]int{
			fmt.Sprintf("目标 %s/ssrf/in-get 可能存在SSRF漏洞", vulAddr): 1,
			fmt.Sprintf("目标 %s/ssrf/safe 可能存在SSRF漏洞", vulAddr):   0,
		},
		StrictMode: true,
	}

	vulPost := VulInfo{
		Path: []string{
			"/ssrf/in-post",
		},
		Method: "POST",
		Headers: []*ypb.KVPair{
			{
				Key:   "Content-Type",
				Value: "application/x-www-form-urlencoded",
			},
		},
		Body: []byte(`url=`),
		ExpectedResult: map[string]int{
			fmt.Sprintf("目标 %s/ssrf/in-post 可能存在SSRF漏洞", vulAddr): 1,
		},
		StrictMode: true,
	}

	// GET参数测试，3次重试
	var getSuccess bool
	for i := 0; i < 3; i++ {
		getSuccess = CoreMitmPlugTest(pluginName, server, vulGet, client, t)
		if getSuccess {
			break
		}
		if i < 2 {
			t.Logf("SSRF GET测试第%d次失败，正在重试...", i+1)
		}
	}
	Must(getSuccess, "SSRF HTTP Public插件对于 GET 参数检测不符合预期")

	// POST参数测试，3次重试
	var postSuccess bool
	for i := 0; i < 3; i++ {
		postSuccess = CoreMitmPlugTest(pluginName, server, vulPost, client, t)
		if postSuccess {
			break
		}
		if i < 2 {
			t.Logf("SSRF POST测试第%d次失败，正在重试...", i+1)
		}
	}
	Must(postSuccess, "SSRF HTTP Public插件对于 POST 参数检测不符合预期")
}

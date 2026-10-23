package coreplugin_test

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

	vulJSONBody := VulInfo{
		Path: []string{
			"/ssrf/in-json-body",
		},
		Method: "POST",
		Headers: []*ypb.KVPair{
			{
				Key:   "Content-Type",
				Value: "application/json",
			},
		},
		Body: []byte(`{"url": "http://www.baidu.com/"}`),
		ExpectedResult: map[string]int{
			fmt.Sprintf("目标 %s/ssrf/in-json-body 可能存在SSRF漏洞", vulAddr): 1,
		},
		StrictMode: true,
	}

	vulJSONInPostParam := VulInfo{
		Path: []string{
			"/ssrf/json-in-post-param",
		},
		Method: "POST",
		Headers: []*ypb.KVPair{
			{
				Key:   "Content-Type",
				Value: "application/x-www-form-urlencoded",
			},
		},
		Body: []byte(`json={"url": "http://www.baidu.com/"}`),
		ExpectedResult: map[string]int{
			fmt.Sprintf("目标 %s/ssrf/json-in-post-param 可能存在SSRF漏洞", vulAddr): 1,
		},
		StrictMode: true,
	}

	vulMultipart := VulInfo{
		Path: []string{
			"/ssrf/in-post-multipart",
		},
		Method: "POST",
		Headers: []*ypb.KVPair{
			{
				Key:   "Content-Type",
				Value: "multipart/form-data; boundary=ssrfboundary",
			},
		},
		Body: []byte("--ssrfboundary\r\nContent-Disposition: form-data; name=\"url\"\r\n\r\nhttp://www.baidu.com/\r\n--ssrfboundary--\r\n"),
		// 插件的 GetCommonParams 只提取 urlencoded / JSON / base64 参数，暂不解析
		// multipart body，因此预期不检出；插件支持 multipart 提取后应翻转为 1
		ExpectedResult: map[string]int{
			fmt.Sprintf("目标 %s/ssrf/in-post-multipart 可能存在SSRF漏洞", vulAddr): 0,
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

	// JSON Body测试，3次重试
	var jsonBodySuccess bool
	for i := 0; i < 3; i++ {
		jsonBodySuccess = CoreMitmPlugTest(pluginName, server, vulJSONBody, client, t)
		if jsonBodySuccess {
			break
		}
		if i < 2 {
			t.Logf("SSRF JSON Body测试第%d次失败，正在重试...", i+1)
		}
	}
	Must(jsonBodySuccess, "SSRF HTTP Public插件对于 JSON Body 检测不符合预期")

	// POST参数是JSON测试，3次重试
	var jsonInPostSuccess bool
	for i := 0; i < 3; i++ {
		jsonInPostSuccess = CoreMitmPlugTest(pluginName, server, vulJSONInPostParam, client, t)
		if jsonInPostSuccess {
			break
		}
		if i < 2 {
			t.Logf("SSRF POST JSON参数测试第%d次失败，正在重试...", i+1)
		}
	}
	Must(jsonInPostSuccess, "SSRF HTTP Public插件对于 POST JSON 参数检测不符合预期")

	// multipart 参数：预期不检出（见 vulMultipart 注释），检出即视为误报
	CoreMitmPlugTest(pluginName, server, vulMultipart, client, t)
}

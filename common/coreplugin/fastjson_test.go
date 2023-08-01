package coreplugin

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_Fastjson(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}
	log.Infof("vulAddr: %v", vulAddr)
	//time.Sleep(5 * time.Hour)
	pluginName := "Fastjson 综合检测"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}
	vulInGet := VulInfo{
		Path: []string{
			"/fastjson/json-in-query?auth=" + codec.EncodeUrlCode(`{"user":"admin","password":"password"}`) + "&action=login",
		},
		ExpectedResult: map[string]int{
			"目标 fastjson 框架可能存在 RCE 漏洞 (DNSLog Check)": 1,
		},
		StrictMode: false,
	}
	Must(TestCoreMitmPlug(pluginName, server, vulInGet, client, t), "Fastjson 综合检测插件检测结果不符合预期")
	vulInForm := VulInfo{
		Method: "POST",
		Path: []string{
			"/fastjson/json-in-form",
		},
		Headers: []*ypb.KVPair{
			{
				Key:   "Content-Type",
				Value: "application/x-www-form-urlencoded",
			},
		},
		Body: []byte(`auth={"user":"admin","password":"password"}`),
		ExpectedResult: map[string]int{
			"目标 fastjson 框架可能存在 RCE 漏洞 (DNSLog Check)": 1,
		},
		StrictMode: false,
	}
	Must(TestCoreMitmPlug(pluginName, server, vulInForm, client, t), "Fastjson 综合检测插件检测结果不符合预期")
	vulInBodyJson := VulInfo{
		Method: "POST",
		Path: []string{
			"/fastjson/json-in-body",
		},
		Body: []byte(`{"user":"admin","password":"password"}`),
		Headers: []*ypb.KVPair{
			{
				Key:   "Content-Type",
				Value: "application/json",
			},
		},
		ExpectedResult: map[string]int{
			"目标 fastjson 框架可能存在 RCE 漏洞 (DNSLog Check)": 1,
		},
		StrictMode: true,
	}
	Must(TestCoreMitmPlug(pluginName, server, vulInBodyJson, client, t), "Fastjson 综合检测插件检测结果不符合预期")
	vulInGetServeByJackson := VulInfo{ // 这里不应该检出任何漏洞，并且发包数量应该为 1
		Method: "GET",
		Path: []string{
			"/fastjson/jackson-in-query?auth=" + codec.EncodeUrlCode(`{"user":"admin","password":"password"}`) + "&action=login",
		},
		ExpectedResult: map[string]int{},
		StrictMode:     true,
	}
	Must(TestCoreMitmPlug(pluginName, server, vulInGetServeByJackson, client, t), "Fastjson 综合检测插件检测结果不符合预期")
	// TODO: 需要先修复 fuzz 请求出错后不能获取Duration的问题
	//vulInGetIntranet := VulInfo{
	//	Method: "GET",
	//	Path: []string{
	//		"/fastjson/get-in-query-intranet?auth=" + codec.EncodeUrlCode(`{"user":"admin","password":"password"}`) + "&action=login",
	//	},
	//	ExpectedResult: map[string]int{
	//		"目标 fastjson 框架可能存在 RCE 漏洞 (Delay Check)": 1,
	//	},
	//	StrictMode: true,
	//}
	//Must(TestCoreMitmPlug(pluginName, server, vulInGetIntranet, client, t), "Fastjson 综合检测插件检测结果不符合预期")
	// TODO: Cookie Fuzz 需要支持自动解码
	//vulInGet := VulInfo{
	//	Method: "GET",
	//	Path: []string{
	//		"/fastjson/json-in-cookie?action=login",
	//	},
	//	Headers: []*ypb.KVPair{
	//		{
	//			Key:   "Cookie",
	//			Value: `auth=` + codec.EncodeBase64Url(`{"id":"-1"}`),
	//		},
	//	},
	//	ExpectedResult: map[string]int{
	//		"目标 fastjson 框架可能存在 RCE 漏洞 (DNSLog Check)": 1,
	//	},
	//	StrictMode: true,
	//}
	//Must(TestCoreMitmPlug(pluginName, server, vulInGet, client, t), "Fastjson 综合检测插件检测结果不符合预期")
	// TODO: Authorization Fuzz 需要支持自动解码
	//vulInGet := VulInfo{
	//	Method: "GET",
	//	Path: []string{
	//		"/fastjson/json-in-authorization?action=login",
	//	},
	//	Headers: []*ypb.KVPair{
	//		{
	//			Key:   "Authorization",
	//			Value: `Basic ` + codec.EncodeBase64Url(`{"user":"admin","password":"password"}`),
	//		},
	//	},
	//	ExpectedResult: map[string]int{
	//		"目标 fastjson 框架可能存在 RCE 漏洞 (DNSLog Check)": 1,
	//	},
	//	StrictMode: true,
	//}
	//Must(TestCoreMitmPlug(pluginName, server, vulInGet, client, t), "Fastjson 综合检测插件检测结果不符合预期")
}
func TestFastjson(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}
	log.Infof("vulAddr: %v", vulAddr)
	//time.Sleep(5 * time.Hour)
	pluginName := "Fastjson 综合检测"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}
	vulInGet := VulInfo{
		Method: "GET",
		Path: []string{
			"/fastjson/get-in-query-intranet?auth=" + codec.EncodeUrlCode(`{"user":"admin","password":"password"}`) + "&action=login",
		},
		ExpectedResult: map[string]int{
			"目标 fastjson 框架可能存在 RCE 漏洞 (Delay Check)": 1,
		},
		StrictMode: true,
	}
	Must(TestCoreMitmPlug(pluginName, server, vulInGet, client, t), "Fastjson 综合检测插件检测结果不符合预期")
}

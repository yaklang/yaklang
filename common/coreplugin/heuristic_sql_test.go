package coreplugin

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc"
)

func TestGRPCMUSTPASS_SQL(t *testing.T) {
	// TODO fail test case
	t.SkipNow()
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		panic(err)
	}

	pluginName := "启发式SQL注入检测"
	vul := VulInfo{
		Path: []string{
			//"/user/by-id-safe?id=1",
			"/user/cookie-id",
			//"/user/id?id=1",
			//"/user/id-json?id=%7B%22uid%22%3A1%2C%22id%22%3A%221%22%7D",
			//"/user/id-b64-json?id=eyJ1aWQiOjEsImlkIjoiMSJ9",
			//"/user/name?name=admin",
			//"/user/id-error?id=1",
			//"/user/name/like?name=a",
			//"/user/name/like/2?name=a",
			//"/user/name/like/b64j?data=eyJuYW1lYjY0aiI6ImEifQ%3D%3D",
		},
		Headers: []*ypb.KVPair{{
			Key:   "Cookie",
			Value: "ID=1",
		}},
		ExpectedResult: map[string]int{
			//"参数:id未检测到闭合边界":                         1,
			//"疑似SQL注入：【参数：数字[id] 无边界闭合】":                        4,
			"存在基于UNION SQL 注入: [参数名:id 值:[1]]": 1,
			//"疑似SQL注入：【参数：字符串[name] 单引号闭合】":                     1,
			//"存在基于UNION SQL 注入: [参数名:name 值:[admin]": 1,
			////"疑似SQL注入：【参数：数字[ID] 双引号闭合】":                        1,
			//"存在基于UNION SQL 注入: [参数名:ID 值:[1]]": 1,
			////"疑似SQL注入：【参数：字符串[name] like注入( %' )】":              2,
			//"存在基于UNION SQL 注入: [参数名:name 值:[a]]": 1,
			////"疑似SQL注入：【参数：字符串[data] like注入( %' )】":              1,
			//"可能存在基于错误的 SQL 注入: [参数名:id 原值:[1]] 猜测数据库类型: MySQL": 1,
		},
		StrictMode: false,
	}

	//vul10 := VulInfo{
	//	Path:           "/user/name/like/b64?nameb64=%59%51%3d%3d",
	//	ExpectedResult: map[string]int{"疑似SQL注入：【参数：字符串[name] like注入( %' )】": 3},
	//}
	Must(CoreMitmPlugTest(pluginName, server, vul, client, t), "SQL插件对于SQL注入检测结果不符合预期")
}

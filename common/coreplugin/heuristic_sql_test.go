package coreplugin

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc"
)

func TestGRPCMUSTPASS_SQL(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	pluginName := "启发式SQL注入检测"
	tests := []struct {
		name           string
		path           string
		header         []*ypb.KVPair
		expectedResult map[string]int
	}{
		{
			name: "Safe ID",
			path: "/user/by-id-safe?id=1",
			expectedResult: map[string]int{
				"": 0,
			},
		},
		{
			name: "Cookie Skip",
			path: "/user/cookie-id",
			expectedResult: map[string]int{
				"存在基于UNION SQL 注入: [参数名:ID 值:1]": 1,
				//"疑似SQL注入：【参数：数字[ID] 双引号闭合】":        1,
			},
			header: []*ypb.KVPair{
				{
					Key:   "Cookie",
					Value: "ID=1",
				},
			},
		},
		{
			name: "Numeric ID",
			path: "/user/id?id=1",
			expectedResult: map[string]int{
				"存在基于UNION SQL 注入: [参数名:id 值:1]": 1,
				//疑似SQL注入：【参数：数字[id] 无边界闭合】
			},
		},
		{
			name: "JSON ID",
			path: "/user/id-json?id=%7B%22uid%22%3A1%2C%22id%22%3A%221%22%7D",
			expectedResult: map[string]int{
				"存在基于UNION SQL 注入: [参数名:id 值:1]": 1,
				// 疑似SQL注入：【参数：数字[id] 无边界闭合】
			},
		},
		{
			name: "Base64 JSON ID",
			path: "/user/id-b64-json?id=eyJ1aWQiOjEsImlkIjoiMSJ9",
			expectedResult: map[string]int{
				"存在基于UNION SQL 注入: [参数名:id 值:1]": 1,
				// 疑似SQL注入：【参数：数字[id] 无边界闭合】
			},
		},
		{
			name: "Admin Name",
			path: "/user/name?name=admin",
			expectedResult: map[string]int{
				"存在基于UNION SQL 注入: [参数名:name 值:admin]": 1,
				// 疑似SQL注入：【参数：字符串[name] 单引号闭合】
			},
		},
		{
			name: "Error ID",
			path: "/user/id-error?id=1",
			expectedResult: map[string]int{
				"可能存在基于错误的 SQL 注入: [参数名:id 原值:1] 猜测数据库类型: MySQL": 1,
				"存在基于UNION SQL 注入: [参数名:id 值:1]":                 1,
				// 疑似SQL注入：【参数：数字[id] 无边界闭合】
			},
		},
		{
			name: "Like Name",
			path: "/user/name/like?name=a",
			expectedResult: map[string]int{
				"存在基于UNION SQL 注入: [参数名:name 值:a]": 1,
				// 疑似SQL注入：【参数：字符串[name] like注入( %' )】
			},
		},
		{
			name: "Like Name 2",
			path: "/user/name/like/2?name=a",
			expectedResult: map[string]int{
				"": 0,
				// 疑似SQL注入：【参数：字符串[name] like注入( %' )】
			},
		},
		{
			name: "Base64 JSON Like Name",
			path: "/user/name/like/b64j?data=eyJuYW1lYjY0aiI6ImEifQ%3D%3D",
			expectedResult: map[string]int{
				"": 0,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vul := VulInfo{
				Path:           []string{tc.path},
				ExpectedResult: tc.expectedResult,
				StrictMode:     false,
				Headers:        tc.header,
			}
			Must(CoreMitmPlugTest(pluginName, server, vul, client, t))
		})
	}
}

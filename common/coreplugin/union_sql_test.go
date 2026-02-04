package coreplugin_test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/yakgrpc"
)

func TestGRPCMUSTPASS_SQLUnion(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	pluginName := "SQL注入-UNION注入-MD5函数"
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
				"SQL注入（UNION）列数(MD5)[9] 参数[ID]": 1,
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
				"SQL注入（UNION）列数(MD5)[9] 参数[id]": 1,
			},
		},
		{
			name: "JSON ID",
			path: "/user/id-json?id=%7B%22uid%22%3A1%2C%22id%22%3A%221%22%7D",
			expectedResult: map[string]int{
				"SQL注入（UNION）列数(MD5)[9] 参数[id]": 1,
			},
		},
		{
			name: "Base64 JSON ID",
			path: "/user/id-b64-json?id=eyJ1aWQiOjEsImlkIjoiMSJ9",
			expectedResult: map[string]int{
				"SQL注入（UNION）列数(MD5)[9] 参数[id]": 1,
			},
		},
		{
			name: "Admin Name",
			path: "/user/name?name=admin",
			expectedResult: map[string]int{
				"SQL注入（UNION）列数(MD5)[9] 参数[name]": 1,
			},
		},
		{
			name: "Error ID",
			path: "/user/id-error?id=1",
			expectedResult: map[string]int{
				"SQL注入（UNION）列数(MD5)[9] 参数[id]": 1,
			},
		},
		{
			name: "Like Name",
			path: "/user/name/like?name=a",
			expectedResult: map[string]int{
				"SQL注入（UNION）列数(MD5)[9] 参数[name]": 1,
			},
		},
		{
			name: "Like Name 2",
			path: "/user/name/like/2?name=a",
			expectedResult: map[string]int{
				"SQL注入（UNION）列数(MD5)[9] 参数[name]": 1,
			},
		},
		{
			name: "Base64 JSON Like Name",
			path: "/user/name/like/b64j?data=eyJuYW1lYjY0aiI6ImEifQ%3D%3D",
			expectedResult: map[string]int{
				"SQL注入（UNION）列数(MD5)[9] 参数[data]": 1,
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

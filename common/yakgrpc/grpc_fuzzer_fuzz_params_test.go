package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"testing"
	"time"
)

func TestFuzzerFuzzParams(t *testing.T) {
	type testCase struct {
		name          string
		mutateMethods []*ypb.MutateMethod
		// 预期的Cookie和Header验证逻辑
		expectedCookies map[string][]string // Cookie名称到期望值的映射
		expectedHeaders map[string]string   // Header名称到期望值的映射
		expectedCount   int                 // 预期的请求数量
	}

	tests := []testCase{
		{
			name: "修改原有字段",
			mutateMethods: []*ypb.MutateMethod{
				{
					Type: "Cookie",
					Value: []*ypb.KVPair{
						{Key: "a", Value: "xxxx"},
					},
				},
				{
					Type: "Headers",
					Value: []*ypb.KVPair{
						{Key: "Cookie", Value: "kkk=vvvv;"},
					},
				},
			},
			expectedCookies: map[string][]string{
				"a": {"1", "xxxx"}, "kkk": {"vvvv"}, "b": {"2"},
			},
			expectedHeaders: map[string]string{}, // 如果有特定header预期，添加到这里
			expectedCount:   3,
		},
		{
			name: "追加key",
			mutateMethods: []*ypb.MutateMethod{
				{
					Type: "Cookie",
					Value: []*ypb.KVPair{
						{Key: "cc", Value: "zz"},
					},
				},
				{
					Type: "Headers",
					Value: []*ypb.KVPair{
						{Key: "Echo", Value: "whoami"},
					},
				},
			},
			expectedCookies: map[string][]string{
				"a": {"1"}, "b": {"2"}, "cc": {"zz"},
			},
			expectedHeaders: map[string]string{"Echo": "whoami"}, // 如果有特定header预期，添加到这里
			expectedCount:   3,
		},
		{
			name: "修改 && 追加key",
			mutateMethods: []*ypb.MutateMethod{
				{
					Type: "Cookie",
					Value: []*ypb.KVPair{
						{Key: "cc", Value: "zz"},
					},
				},
				{
					Type: "Cookie",
					Value: []*ypb.KVPair{
						{Key: "a", Value: "pp"},
					},
				},
			},
			expectedCookies: map[string][]string{
				"a": {"1", "pp"}, "b": {"2"}, "cc": {"zz"},
			},
			expectedCount: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK

aaa`))

			client, err := NewLocalClient()
			if err != nil {
				t.Fatal(err)
			}

			target := utils.HostPort(host, port)

			raw := `POST / HTTP/1.1
Host: %s
Cookie: a=1; b=2;
`

			recv, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
				Request:       fmt.Sprintf(raw, target),
				MutateMethods: tc.mutateMethods,
			})
			if err != nil {
				t.Fatal(err)
			}

			allRequests := make([]*http.Request, 0)

			// 执行测试逻辑
			for {
				resp, err := recv.Recv()
				if err != nil {
					break
				}

				req, err := utils.ReadHTTPRequestFromBytes(resp.GetRequestRaw())
				if err != nil {
					t.Fatal("Failed to read HTTP request from bytes:", err)
				}
				allRequests = append(allRequests, req)
			}

			if len(allRequests) != tc.expectedCount {
				t.Fatalf("Expected %d requests, got %d", tc.expectedCount, len(allRequests))
			}

			// 验证Cookies
			verifiedCookies := make(map[string]bool)
			for _, req := range allRequests {
				for name, expectedValues := range tc.expectedCookies {
					cookie, err := req.Cookie(name)
					if err != nil {
						continue // 某些请求可能不包含特定cookie
					}

					for _, expectedValue := range expectedValues {
						if cookie.Value == expectedValue {
							verifiedCookies[name+":"+expectedValue] = true
						}
					}
				}
			}

			// 确保所有预期的Cookies都被验证
			for name, expectedValues := range tc.expectedCookies {
				for _, expectedValue := range expectedValues {
					if !verifiedCookies[name+":"+expectedValue] {
						t.Errorf("Cookie %s with expected value %s was not verified", name, expectedValue)
					}
				}
			}

			// 验证Headers
			verifiedHeaders := make(map[string]bool)
			for _, req := range allRequests {
				for name, expectedValue := range tc.expectedHeaders {
					if req.Header.Get(name) == expectedValue {
						verifiedHeaders[name+":"+expectedValue] = true
					}
				}
			}

			// 确保所有预期的Headers都被验证
			for name, expectedValue := range tc.expectedHeaders {
				if !verifiedHeaders[name+":"+expectedValue] {
					t.Errorf("Header %s with expected value %s was not verified", name, expectedValue)
				}
			}
		})
	}
}

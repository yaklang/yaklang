package yakgrpc

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_Basic 测试 HTTPFuzzer 中请求相关的提取器
func TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_Basic(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	tests := []struct {
		name             string
		extractorType    string
		extractorScope   string
		extractorGroups  []string
		regexpMatchGroup []int64
		requestTemplate  string
		expectedKey      string
		expectedValue    string
	}{
		{
			name:             "提取请求头中的 Token",
			extractorType:    "regex",
			extractorScope:   "request_header",
			extractorGroups:  []string{`Bearer\s+([a-zA-Z0-9]+)`},
			regexpMatchGroup: []int64{1}, // 使用捕获组 1
			requestTemplate: `GET /api/test HTTP/1.1
Host: %s
Authorization: Bearer abc123token456
User-Agent: TestClient/1.0

`,
			expectedKey:   "token",
			expectedValue: "abc123token456",
		},
		{
			name:             "提取请求体中的用户名",
			extractorType:    "regex",
			extractorScope:   "request_body",
			extractorGroups:  []string{`username=([^&]+)`},
			regexpMatchGroup: []int64{1}, // 使用捕获组 1
			requestTemplate: `POST /api/login HTTP/1.1
Host: %s
Content-Type: application/x-www-form-urlencoded
Content-Length: 42

username=testuser&password=pass123&age=25`,
			expectedKey:   "username",
			expectedValue: "testuser",
		},
		{
			name:             "提取请求 URL 中的参数",
			extractorType:    "regex",
			extractorScope:   "request_url",
			extractorGroups:  []string{`id=(\d+)`},
			regexpMatchGroup: []int64{1}, // 使用捕获组 1
			requestTemplate: `GET /api/user?id=12345&action=view HTTP/1.1
Host: %s

`,
			expectedKey:   "user_id",
			expectedValue: "12345",
		},
		{
			name:             "提取请求原始数据中的路径",
			extractorType:    "regex",
			extractorScope:   "request_raw",
			extractorGroups:  []string{`GET\s+(/api/[^\s]+)`},
			regexpMatchGroup: []int64{1}, // 使用捕获组 1
			requestTemplate: `GET /api/products/electronics HTTP/1.1
Host: %s

`,
			expectedKey:   "api_path",
			expectedValue: "/api/products/electronics",
		},
		{
			name:            "使用 kval 提取请求体中的键值对",
			extractorType:   "kval",
			extractorScope:  "request_body",
			extractorGroups: []string{"password"},
			requestTemplate: `POST /api/login HTTP/1.1
Host: %s
Content-Type: application/x-www-form-urlencoded

username=admin&password=secret123&remember=true`,
			expectedKey:   "password_data",
			expectedValue: "secret123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var extractedValue string
			verified := false

			// 创建 mock HTTP 服务器
			host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(200)
				writer.Write([]byte(`{"status":"ok"}`))
			})
			addr := utils.HostPort(host, port)

			// 构造请求
			requestRaw := fmt.Sprintf(tt.requestTemplate, addr)

			// 执行 HTTPFuzzer
			stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
				Request:                  requestRaw,
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				Extractors: []*ypb.HTTPResponseExtractor{
					{
						Name:             tt.expectedKey,
						Type:             tt.extractorType,
						Scope:            tt.extractorScope,
						Groups:           tt.extractorGroups,
						RegexpMatchGroup: tt.regexpMatchGroup,
					},
				},
			})
			require.NoError(t, err)

			// 接收响应并验证提取结果
			for {
				resp, err := stream.Recv()
				if err != nil {
					break
				}
				if resp == nil {
					break
				}

				// 检查提取结果
				if len(resp.ExtractedResults) > 0 {
					for _, kv := range resp.ExtractedResults {
						if kv.Key == tt.expectedKey {
							extractedValue = string(kv.Value)
							verified = true
							break
						}
					}
				}
			}

			assert.True(t, verified, "应该提取到值")
			assert.Equal(t, tt.expectedValue, extractedValue, "提取的值应该匹配预期")
		})
	}
}

// TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_JSON 测试从请求体中提取 JSON 数据
func TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_JSON(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	var extractedEmail, extractedName string
	verified := false

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		writer.Write([]byte(`{"status":"created"}`))
	})
	addr := utils.HostPort(host, port)

	requestRaw := fmt.Sprintf(`POST /api/register HTTP/1.1
Host: %s
Content-Type: application/json

{"email":"test@example.com","name":"John Doe","age":30}`, addr)

	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:                  requestRaw,
		IsHTTPS:                  false,
		PerRequestTimeoutSeconds: 5,
		Extractors: []*ypb.HTTPResponseExtractor{
			{
				Name:   "email",
				Type:   "json",
				Scope:  "request_body",
				Groups: []string{".email"},
			},
			{
				Name:   "name",
				Type:   "json",
				Scope:  "request_body",
				Groups: []string{".name"},
			},
		},
	})
	require.NoError(t, err)

	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if resp == nil {
			break
		}

		if len(resp.ExtractedResults) > 0 {
			for _, kv := range resp.ExtractedResults {
				if kv.Key == "email" {
					extractedEmail = string(kv.Value)
				}
				if kv.Key == "name" {
					extractedName = string(kv.Value)
				}
			}
			verified = true
		}
	}

	assert.True(t, verified, "应该提取到 JSON 值")
	assert.Equal(t, "test@example.com", extractedEmail, "提取的 email 应该正确")
	assert.Equal(t, "John Doe", extractedName, "提取的 name 应该正确")
}

// TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_Chained 测试链式提取（先从请求提取，再在后续请求中使用）
func TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_Chained(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	firstRequestDone := false

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		writer.Write([]byte(`{"status":"ok"}`))
	})
	addr := utils.HostPort(host, port)

	// 第一个请求：发送带 token 的请求，并提取 token
	requestRaw1 := fmt.Sprintf(`GET /api/test HTTP/1.1
Host: %s
Authorization: Bearer mytoken123

`, addr)

	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:                  requestRaw1,
		IsHTTPS:                  false,
		PerRequestTimeoutSeconds: 5,
		Extractors: []*ypb.HTTPResponseExtractor{
			{
				Name:             "extracted_token",
				Type:             "regex",
				Scope:            "request_header",
				Groups:           []string{`Bearer\s+(\w+)`},
				RegexpMatchGroup: []int64{1}, // 使用捕获组 1
			},
		},
	})
	require.NoError(t, err)

	var extractedToken string
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if resp == nil {
			break
		}

		if len(resp.ExtractedResults) > 0 {
			for _, kv := range resp.ExtractedResults {
				if kv.Key == "extracted_token" {
					extractedToken = string(kv.Value)
					firstRequestDone = true
				}
			}
		}
	}

	assert.True(t, firstRequestDone, "第一个请求应该完成")
	assert.Equal(t, "mytoken123", extractedToken, "应该提取到正确的 token")
}

// TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_NucleiDSL 测试使用 nuclei-dsl 提取请求数据
func TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_NucleiDSL(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	tests := []struct {
		name          string
		dslExpr       string
		expectedKey   string
		checkContains bool
		expectedValue string
	}{
		{
			name:          "提取 request_url",
			dslExpr:       "request_url",
			expectedKey:   "url_value",
			checkContains: true,
			expectedValue: "/api/v2/users",
		},
		{
			name:          "提取 request_body",
			dslExpr:       "request_body",
			expectedKey:   "body_value",
			checkContains: true,
			expectedValue: "userId",
		},
		{
			name:          "提取 request_headers",
			dslExpr:       "request_headers",
			expectedKey:   "headers_value",
			checkContains: true,
			expectedValue: "Content-Type",
		},
		{
			name:          "组合请求和响应数据",
			dslExpr:       `concat(request_body, " -> ", body)`,
			expectedKey:   "combined",
			checkContains: true,
			expectedValue: "userId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var extractedValue string
			verified := false

			host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(200)
				writer.Write([]byte(`{"status":"success"}`))
			})
			addr := utils.HostPort(host, port)

			requestRaw := fmt.Sprintf(`POST /api/v2/users HTTP/1.1
Host: %s
Content-Type: application/json

{"userId":12345}`, addr)

			stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
				Request:                  requestRaw,
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				Extractors: []*ypb.HTTPResponseExtractor{
					{
						Name:   tt.expectedKey,
						Type:   "nuclei-dsl",
						Scope:  "raw",
						Groups: []string{tt.dslExpr},
					},
				},
			})
			require.NoError(t, err)

			for {
				resp, err := stream.Recv()
				if err != nil {
					break
				}
				if resp == nil {
					break
				}

				if len(resp.ExtractedResults) > 0 {
					for _, kv := range resp.ExtractedResults {
						if kv.Key == tt.expectedKey {
							extractedValue = string(kv.Value)
							verified = true
						}
					}
				}
			}

			assert.True(t, verified, "应该提取到值")
			if tt.checkContains {
				assert.Contains(t, extractedValue, tt.expectedValue, "提取的值应该包含预期内容")
			} else {
				assert.Equal(t, tt.expectedValue, extractedValue, "提取的值应该完全匹配")
			}
		})
	}
}

// TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_EmptyScope 测试当 scope 为空时应该默认处理响应
// 注意：空 scope 默认从响应整体（包含header和body）提取
func TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_EmptyScope(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	var extractedValue string
	verified := false

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		writer.Write([]byte(`{"response_key":"response_value"}`))
	})
	addr := utils.HostPort(host, port)

	requestRaw := fmt.Sprintf(`GET /api/test HTTP/1.1
Host: %s

`, addr)

	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:                  requestRaw,
		IsHTTPS:                  false,
		PerRequestTimeoutSeconds: 5,
		Extractors: []*ypb.HTTPResponseExtractor{
			{
				Name:             "status_code",
				Type:             "regex",
				Scope:            "", // 空 scope，应该默认从整个响应（header+body）中提取
				Groups:           []string{`HTTP/1\.\d+\s+(\d+)`},
				RegexpMatchGroup: []int64{1}, // 提取状态码
			},
		},
	})
	require.NoError(t, err)

	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if resp == nil {
			break
		}

		if len(resp.ExtractedResults) > 0 {
			for _, kv := range resp.ExtractedResults {
				if kv.Key == "status_code" {
					extractedValue = string(kv.Value)
					verified = true
				}
			}
		}
	}

	assert.True(t, verified, "应该提取到响应值")
	// 空 scope 时，从整个响应（header+body）提取，应该能匹配到状态码
	assert.Equal(t, "200", extractedValue, "应该从整个响应中提取到状态码")
}

// TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_MultipleValues 测试提取多个值
func TestGRPCMUSTPASS_HTTPFuzzer_RequestExtractor_MultipleValues(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	var extractedValues []string
	verified := false

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		writer.Write([]byte(`OK`))
	})
	addr := utils.HostPort(host, port)

	requestRaw := fmt.Sprintf(`GET /api/test HTTP/1.1
Host: %s
X-Custom-Header: value1, value2, value3

`, addr)

	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:                  requestRaw,
		IsHTTPS:                  false,
		PerRequestTimeoutSeconds: 5,
		Extractors: []*ypb.HTTPResponseExtractor{
			{
				Name:   "custom_values",
				Type:   "regex",
				Scope:  "request_header",
				Groups: []string{`value\d+`},
			},
		},
	})
	require.NoError(t, err)

	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if resp == nil {
			break
		}

		if len(resp.ExtractedResults) > 0 {
			for _, kv := range resp.ExtractedResults {
				if kv.Key == "custom_values" {
					extractedValues = append(extractedValues, string(kv.Value))
					verified = true
				}
			}
		}
	}

	assert.True(t, verified, "应该提取到值")
	// 应该提取到多个匹配的值
	assert.True(t, len(extractedValues) > 0, "应该至少提取到一个值")
}

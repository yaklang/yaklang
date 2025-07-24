package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

type testCase struct {
	aiName         string
	expectedURI    string
	expectedDomain string
}

func TestLoadOption(t *testing.T) {
	aispec.EnableNewLoadOption = true

	// 定义测试用例，包含AI类型、预期URI和预期域名
	testCases := []testCase{
		// {"openai", "/v1/chat/completions", "api.openai.com"},
		{"chatglm", "/api/paas/v4/chat/completions", "open.bigmodel.cn"},
		{"moonshot", "/v1/chat/completions", "api.moonshot.cn"},
		{"tongyi", "/compatible-mode/v1/chat/completions", "dashscope.aliyuncs.com"},
		{"deepseek", "/chat/completions", "api.deepseek.com"},
		{"siliconflow", "/v1/chat/completions", "api.siliconflow.cn"},
		{"openrouter", "/api/v1/chat/completions", "openrouter.ai"},
		{"aibalance", "/v1/chat/completions", "aibalance.yaklang.com"},
	}

	var receivedURI, receivedHost string
	var isRecivedRequest bool
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		receivedURI = lowhttp.GetHTTPRequestPath(req)
		receivedHost = lowhttp.GetHTTPPacketHeader(req, "Host")
		isRecivedRequest = true
		return nil
	})

	testKey := utils.RandStringBytes(10) + "." + utils.RandStringBytes(10)
	// 测试所有AI网关
	for _, tc := range testCases {
		t.Run(tc.aiName, func(t *testing.T) {
			aiClient, ok := aispec.Lookup(tc.aiName)
			if !ok {
				t.Skipf("AI gateway %s not found, skipping test", tc.aiName)
				return
			}
			isRecivedRequest = false

			// 测试1: 不加domain配置，应该使用默认域名和URI
			t.Run("default_config", func(t *testing.T) {
				aiClient.LoadOption(
					aispec.WithHost(host),
					aispec.WithPort(port),
					aispec.WithAPIKey(testKey),
					aispec.WithNoHttps(true),
				)
				aiClient.Chat("hello")

				// 验证是否收到请求
				assert.True(t, isRecivedRequest, "Expected to receive request for %s", tc.aiName)

				// 验证URI应该是预期的默认URI
				assert.Equal(t, tc.expectedURI, receivedURI, "Expected URI %s, got %s for %s", tc.expectedURI, receivedURI, tc.aiName)

				// 验证Host应该是预期的默认域名
				assert.Equal(t, tc.expectedDomain, receivedHost, "Expected domain %s, got %s for %s", tc.expectedDomain, receivedHost, tc.aiName)
			})
			isRecivedRequest = false

			// 测试2: 指定自定义域名，应该使用指定的域名
			t.Run("custom_config", func(t *testing.T) {
				customDomain := utils.RandStringBytes(10) + ".com"

				aiClient.LoadOption(
					aispec.WithAPIKey(testKey),
					aispec.WithHost(host),
					aispec.WithPort(port),
					aispec.WithDomain(customDomain),
					aispec.WithNoHttps(true),
				)
				aiClient.Chat("hello")

				// 验证是否收到请求
				assert.True(t, isRecivedRequest, "Expected to receive request for %s", tc.aiName)

				// 验证URI应该是预期的默认URI（因为domain只影响域名，不影响URI路径）
				assert.Equal(t, tc.expectedURI, receivedURI, "Expected URI %s, got %s for %s with custom domain", tc.expectedURI, receivedURI, tc.aiName)

				// 验证Host应该是指定的自定义域名
				assert.Equal(t, customDomain, receivedHost, "Expected custom domain %s, got %s for %s", customDomain, receivedHost, tc.aiName)
			})

			// 测试3: 指定无效的域名
			t.Run("invalid_domain", func(t *testing.T) {
				customDomain := utils.RandStringBytes(10) + ".com"

				aiClient.LoadOption(
					aispec.WithAPIKey(testKey),
					aispec.WithHost(host),
					aispec.WithPort(port),
					aispec.WithDomain("http://"+customDomain),
					aispec.WithNoHttps(false),
				)
				aiClient.Chat("hello")

				// 验证是否收到请求
				assert.True(t, isRecivedRequest, "Expected to receive request for %s", tc.aiName)

				// 验证URI应该是预期的默认URI（因为domain只影响域名，不影响URI路径）
				assert.Equal(t, tc.expectedURI, receivedURI, "Expected URI %s, got %s for %s with custom domain", tc.expectedURI, receivedURI, tc.aiName)

				// 验证Host应该是指定的自定义域名
				assert.Equal(t, customDomain, receivedHost, "Expected custom domain %s, got %s for %s", customDomain, receivedHost, tc.aiName)
			})

			// 测试3: 指定了路径

			t.Run("invalid_domain_with_path", func(t *testing.T) {
				customDomain := utils.RandStringBytes(10) + ".com"
				randUri := "/" + utils.RandStringBytes(10)
				aiClient.LoadOption(
					aispec.WithAPIKey(testKey),
					aispec.WithHost(host),
					aispec.WithPort(port),
					aispec.WithDomain("http://"+customDomain+randUri),
					aispec.WithNoHttps(false),
				)
				aiClient.Chat("hello")

				// 验证是否收到请求
				assert.True(t, isRecivedRequest, "Expected to receive request for %s", tc.aiName)

				// 验证URI应该是预期的默认URI（因为domain只影响域名，不影响URI路径）
				assert.Equal(t, randUri, receivedURI, "Expected URI %s, got %s for %s with custom domain", randUri, receivedURI, tc.aiName)

				// 验证Host应该是指定的自定义域名
				assert.Equal(t, customDomain, receivedHost, "Expected custom domain %s, got %s for %s", customDomain, receivedHost, tc.aiName)
			})
			t.Run("invalid_domain_with_path_and_no_https", func(t *testing.T) {
				customDomain := utils.RandStringBytes(10) + ".com"
				randUri := "/" + utils.RandStringBytes(10)
				aiClient.LoadOption(
					aispec.WithAPIKey(testKey),
					aispec.WithHost(host),
					aispec.WithPort(port),
					aispec.WithDomain(customDomain+randUri),
					aispec.WithNoHttps(true),
				)
				aiClient.Chat("hello")

				// 验证是否收到请求
				assert.True(t, isRecivedRequest, "Expected to receive request for %s", tc.aiName)

				if !strings.HasPrefix(receivedURI, randUri) {
					t.Fatalf("Expected URI %s, got %s for %s with custom domain", randUri, receivedURI, tc.aiName)
				}

				// 验证Host应该是指定的自定义域名
				assert.Equal(t, customDomain, receivedHost, "Expected custom domain %s, got %s for %s", customDomain, receivedHost, tc.aiName)
			})
		})
	}
}

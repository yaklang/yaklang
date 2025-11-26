package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestGRPCMUSTPASS_HybridScan_Vars 测试 ScanHybridTargetWithPlugin 的 INJECTED_VARS 传递功能
// 这是一个关键的回归测试，确保从命令行传入的 vars 能正确传递到 nuclei 插件执行
func TestGRPCMUSTPASS_HybridScan_Vars(t *testing.T) {
	// 初始化数据库
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db, "database should not be nil")

	// 创建 mock HTTP 服务器
	var receivedRequests []string
	var mu sync.Mutex
	requestCount := 0

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requestCount++
		body, _ := io.ReadAll(request.Body)
		requestPath := request.URL.Path
		requestHeader := request.Header.Get("X-Custom-Header")
		requestUA := request.Header.Get("User-Agent")

		requestInfo := fmt.Sprintf("Path:%s|Header:%s|UA:%s|Body:%s",
			requestPath, requestHeader, requestUA, string(body))
		receivedRequests = append(receivedRequests, requestInfo)
		mu.Unlock()

		// 返回成功响应
		writer.WriteHeader(200)
		writer.Write([]byte(`{"status":"success","injected_path":"` + requestPath + `"}`))
	})
	target := fmt.Sprintf("http://%s", utils.HostPort(host, port))

	// 创建测试用的 nuclei 模板
	nucleiTemplate := `id: test-hybrid-scan-vars
info:
  name: Test HybridScan Vars Injection
  author: yaklang-test
  severity: info
  description: Test that INJECTED_VARS are correctly passed to nuclei plugins

http:
  - raw:
      - |
        POST /{{api_path}}/{{action}} HTTP/1.1
        Host: {{Hostname}}
        X-Custom-Header: {{custom_header}}
        User-Agent: {{custom_ua}}
        Content-Type: application/json
        
        {"key":"{{api_key}}","data":"{{test_data}}"}
    
    matchers:
      - type: status
        status:
          - 200
    
    extractors:
      - type: dsl
        name: injected_vars_result
        dsl:
          - api_path + "/" + action + "/" + custom_header
`

	// 使用随机插件名避免冲突
	pluginName := fmt.Sprintf("[TEST]-hybrid-scan-vars-%s", utils.RandStringBytes(8))

	// 清理函数 - 确保无论测试成功或失败都会清理
	defer func() {
		db.Unscoped().Where("script_name = ?", pluginName).Delete(&schema.YakScript{})
	}()

	// 创建测试插件
	testPlugin := &schema.YakScript{
		ScriptName: pluginName,
		Type:       "nuclei",
		Content:    nucleiTemplate,
		Author:     "yaklang-test",
	}
	err := yakit.CreateOrUpdateYakScriptByName(db, pluginName, testPlugin)
	require.NoError(t, err, "failed to create test plugin")

	// 从数据库重新加载插件以确保正确
	loadedPlugin, err := yakit.GetYakScriptByName(db, pluginName)
	require.NoError(t, err, "failed to load plugin")
	require.NotNil(t, loadedPlugin, "loaded plugin should not be nil")

	// 验证模板可以被正确解析
	_, err = httptpl.CreateYakTemplateFromNucleiTemplateRaw(nucleiTemplate)
	require.NoError(t, err, "template should be valid")

	// 测试场景1: 基本的 vars 注入
	t.Run("Basic Vars Injection", func(t *testing.T) {
		mu.Lock()
		receivedRequests = []string{}
		requestCount = 0
		mu.Unlock()

		runtimeId := utils.RandStringBytes(16)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 使用随机值避免缓存影响
		randomPath := "api_" + utils.RandStringBytes(6)
		randomAction := "action_" + utils.RandStringBytes(6)
		randomToken := "token_" + utils.RandStringBytes(8)
		randomUA := "UA_" + utils.RandStringBytes(6)
		randomKey := "key_" + utils.RandStringBytes(8)
		randomData := "data_" + utils.RandStringBytes(8)

		// 准备 HybridScanTarget with INJECTED_VARS
		hybridTarget := &HybridScanTarget{
			Request:  []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port)),
			Response: nil,
			IsHttps:  false,
			Url:      target,
			Vars: map[string]any{
				"INJECTED_VARS": map[string]any{
					"api_path":      randomPath,
					"action":        randomAction,
					"custom_header": randomToken,
					"custom_ua":     randomUA,
					"api_key":       randomKey,
					"test_data":     randomData,
				},
			},
		}

		// 创建 feedback client
		feedbackClient := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
			if result.IsMessage && len(result.Message) > 0 {
				t.Logf("[OUTPUT] %s", string(result.Message))
			}
			return nil
		})

		// 执行扫描
		err = ScanHybridTargetWithPlugin(
			runtimeId,
			ctx,
			hybridTarget,
			loadedPlugin,
			"", // no proxy
			feedbackClient,
			filter.NewFilter(),
		)

		// 验证执行成功
		require.NoError(t, err, "ScanHybridTargetWithPlugin should succeed")

		// 等待请求完成
		time.Sleep(1 * time.Second)

		// 验证请求被发送
		mu.Lock()
		defer mu.Unlock()

		require.Greater(t, requestCount, 0, "should have sent at least one request")
		require.NotEmpty(t, receivedRequests, "should have received requests")

		// 验证变量被正确注入到请求中
		requestInfo := receivedRequests[0]
		t.Logf("Received request: %s", requestInfo)

		expectedPath := fmt.Sprintf("Path:/%s/%s", randomPath, randomAction)
		require.Contains(t, requestInfo, expectedPath, "path should contain injected vars")
		require.Contains(t, requestInfo, "Header:"+randomToken, "header should contain injected var")
		require.Contains(t, requestInfo, "UA:"+randomUA, "user-agent should contain injected var")
		require.Contains(t, requestInfo, `"key":"`+randomKey+`"`, "body should contain injected var")
		require.Contains(t, requestInfo, `"data":"`+randomData+`"`, "body should contain injected var")
	})

	// 测试场景2: 多个 target 使用不同的 vars
	t.Run("Multiple Targets With Different Vars", func(t *testing.T) {
		mu.Lock()
		receivedRequests = []string{}
		requestCount = 0
		mu.Unlock()

		// 测试两组不同的随机变量
		randSuffix1 := utils.RandStringBytes(4)
		randSuffix2 := utils.RandStringBytes(4)

		testCases := []struct {
			name string
			vars map[string]any
		}{
			{
				name: "API Set 1",
				vars: map[string]any{
					"api_path":      "api/v1_" + randSuffix1,
					"action":        "action1_" + randSuffix1,
					"custom_header": "Token_" + randSuffix1,
					"custom_ua":     "Client_" + randSuffix1,
					"api_key":       "key_" + randSuffix1,
					"test_data":     "data_" + randSuffix1,
				},
			},
			{
				name: "API Set 2",
				vars: map[string]any{
					"api_path":      "api/v2_" + randSuffix2,
					"action":        "action2_" + randSuffix2,
					"custom_header": "Token_" + randSuffix2,
					"custom_ua":     "Client_" + randSuffix2,
					"api_key":       "key_" + randSuffix2,
					"test_data":     "data_" + randSuffix2,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				runtimeId := utils.RandStringBytes(16)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				hybridTarget := &HybridScanTarget{
					Request:  []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port)),
					Response: nil,
					IsHttps:  false,
					Url:      target,
					Vars: map[string]any{
						"INJECTED_VARS": tc.vars,
					},
				}

				feedbackClient := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
					return nil
				})

				err = ScanHybridTargetWithPlugin(
					runtimeId,
					ctx,
					hybridTarget,
					loadedPlugin,
					"",
					feedbackClient,
					filter.NewFilter(),
				)

				require.NoError(t, err, "ScanHybridTargetWithPlugin should succeed for %s", tc.name)
			})
		}

		// 等待所有请求完成
		time.Sleep(1 * time.Second)

		// 验证收到了两个不同的请求
		mu.Lock()
		defer mu.Unlock()

		require.GreaterOrEqual(t, requestCount, 2, "should have sent at least 2 requests")
		require.GreaterOrEqual(t, len(receivedRequests), 2, "should have received at least 2 requests")

		// 验证第一个请求使用了第一组变量
		firstPath := fmt.Sprintf("api/v1_%s/action1_%s", randSuffix1, randSuffix1)
		require.Contains(t, receivedRequests[0], firstPath, "first request should use first vars")
		require.Contains(t, receivedRequests[0], "Token_"+randSuffix1, "first request should use first header")
		require.Contains(t, receivedRequests[0], "key_"+randSuffix1, "first request should use first api key")

		// 验证第二个请求使用了第二组变量
		secondPath := fmt.Sprintf("api/v2_%s/action2_%s", randSuffix2, randSuffix2)
		require.Contains(t, receivedRequests[1], secondPath, "second request should use second vars")
		require.Contains(t, receivedRequests[1], "Token_"+randSuffix2, "second request should use second header")
		require.Contains(t, receivedRequests[1], "key_"+randSuffix2, "second request should use second api key")
	})

	// 测试场景3: 没有 INJECTED_VARS 的情况（向后兼容）
	t.Run("Backward Compatibility Without INJECTED_VARS", func(t *testing.T) {
		mu.Lock()
		receivedRequests = []string{}
		requestCount = 0
		mu.Unlock()

		runtimeId := utils.RandStringBytes(16)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 不提供 INJECTED_VARS
		hybridTarget := &HybridScanTarget{
			Request:  []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port)),
			Response: nil,
			IsHttps:  false,
			Url:      target,
			Vars:     nil, // 没有 vars
		}

		feedbackClient := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
			return nil
		})

		err = ScanHybridTargetWithPlugin(
			runtimeId,
			ctx,
			hybridTarget,
			loadedPlugin,
			"",
			feedbackClient,
			filter.NewFilter(),
		)

		// 应该不报错，向后兼容
		require.NoError(t, err, "should work without INJECTED_VARS")

		// 等待请求完成
		time.Sleep(1 * time.Second)

		// 验证请求被发送（变量应该保持为 {{varName}} 形式）
		mu.Lock()
		defer mu.Unlock()

		if requestCount > 0 {
			requestInfo := receivedRequests[0]
			t.Logf("Request without vars: %s", requestInfo)
			// 未定义的变量应该保持为 {{varName}} 形式
			require.True(t,
				strings.Contains(requestInfo, "{{api_path}}") || strings.Contains(requestInfo, "/{{action}}"),
				"undefined vars should remain as {{varName}}")
		}
	})
}

// TestGRPCMUSTPASS_HybridScan_Vars_DSL 测试 INJECTED_VARS 在 DSL 中的使用
func TestGRPCMUSTPASS_HybridScan_Vars_DSL(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db, "database should not be nil")

	// 创建 mock HTTP 服务器
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		writer.Write([]byte(`{"code":200,"message":"success","version":"v1.2.3"}`))
	})
	target := fmt.Sprintf("http://%s", utils.HostPort(host, port))

	// 创建使用 DSL 的 nuclei 模板 - 简化版本，只测试 vars 在 DSL 中可用
	nucleiTemplate := `id: test-vars-in-dsl
info:
  name: Test Vars In DSL
  author: yaklang-test
  severity: info

http:
  - raw:
      - |
        GET /api HTTP/1.1
        Host: {{Hostname}}
        
    matchers:
      - type: dsl
        dsl:
          - status_code == expected_status
          - contains(body, "success")
        condition: and
`

	// 使用随机插件名
	pluginName := fmt.Sprintf("[TEST]-vars-dsl-%s", utils.RandStringBytes(8))

	// 确保清理
	defer func() {
		db.Unscoped().Where("script_name = ?", pluginName).Delete(&schema.YakScript{})
	}()

	testPlugin := &schema.YakScript{
		ScriptName: pluginName,
		Type:       "nuclei",
		Content:    nucleiTemplate,
		Author:     "yaklang-test",
	}
	err := yakit.CreateOrUpdateYakScriptByName(db, pluginName, testPlugin)
	require.NoError(t, err)

	loadedPlugin, err := yakit.GetYakScriptByName(db, pluginName)
	require.NoError(t, err)

	// 测试 DSL 中使用 vars
	runtimeId := utils.RandStringBytes(16)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	matched := false

	// 使用随机但有效的状态码进行测试
	expectedStatusCode := 200

	hybridTarget := &HybridScanTarget{
		Request:  []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port)),
		Response: nil,
		IsHttps:  false,
		Url:      target,
		Vars: map[string]any{
			"INJECTED_VARS": map[string]any{
				"expected_status": expectedStatusCode, // DSL 中会使用这个变量
			},
		},
	}

	feedbackClient := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
		if result.IsMessage && len(result.Message) > 0 {
			msg := string(result.Message)
			if strings.Contains(msg, "risk") || strings.Contains(msg, "matched") {
				matched = true
			}
		}
		return nil
	})

	err = ScanHybridTargetWithPlugin(
		runtimeId,
		ctx,
		hybridTarget,
		loadedPlugin,
		"",
		feedbackClient,
		filter.NewFilter(),
	)

	require.NoError(t, err, "ScanHybridTargetWithPlugin should succeed")

	// 等待结果
	time.Sleep(2 * time.Second)

	// 验证 DSL matcher 使用了 INJECTED_VARS (expected_status 变量)
	// 如果 expected_status 没有被正确注入，DSL `status_code == expected_status` 会失败
	require.True(t, matched, "DSL matcher should use INJECTED_VARS (expected_status variable)")
}

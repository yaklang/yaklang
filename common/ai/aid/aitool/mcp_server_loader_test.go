package aitool

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestLoadAIToolFromMCPServers(t *testing.T) {
	// 测试服务器名称
	serverName := "test_sse_server"

	// 测试初始化：清理可能存在的旧数据
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db, "profile database is nil")
	var oldServer schema.MCPServer
	if err := db.Where("name = ?", serverName).First(&oldServer).Error; err == nil {
		db.Unscoped().Delete(&oldServer)
		log.Infof("cleaned up old test mcp server: %s", serverName)
	}

	// 清理函数：测试结束后删除数据库记录
	defer func() {
		db := consts.GetGormProfileDatabase()
		if db != nil {
			var server schema.MCPServer
			if err := db.Where("name = ?", serverName).First(&server).Error; err == nil {
				db.Unscoped().Delete(&server)
				log.Infof("cleaned up test mcp server: %s", serverName)
			}
		}
	}()

	// Step 1: 启动一个真实的 SSE MCP 服务器
	t.Run("UseSSEMCPServer", func(t *testing.T) {
		// 创建底层 MCP 服务器
		mcpServer := server.NewMCPServer(
			"Test MCP Server",
			"1.0.0",
		)

		// 添加一个测试工具
		testTool := mcp.NewTool(
			"test_echo",
			mcp.WithDescription("A simple echo tool for testing"),
			mcp.WithString("message", mcp.Description("The message to echo"), mcp.Required()),
		)

		// 注册工具处理器
		mcpServer.AddTool(testTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			message, ok := request.Params.Arguments["message"].(string)
			if !ok {
				return &mcp.CallToolResult{
					Content: []any{mcp.TextContent{Type: "text", Text: "Error: message must be a string"}},
					IsError: true,
				}, nil
			}
			return &mcp.CallToolResult{
				Content: []any{mcp.TextContent{Type: "text", Text: "Echo: " + message}},
				IsError: false,
			}, nil
		})

		// 获取随机可用端口
		port := utils.GetRandomAvailableTCPPort()
		host := "127.0.0.1"
		hostPort := utils.HostPort(host, port)
		baseURL := fmt.Sprintf("http://%s", hostPort)
		sseURL := baseURL + "/sse"

		log.Infof("Starting SSE MCP server on %s", sseURL)

		// 创建 SSE 服务器
		sseServer := server.NewSSEServer(mcpServer, baseURL)

		// 在后台启动服务器
		serverErrChan := make(chan error, 1)
		go func() {
			err := sseServer.Start(hostPort)
			if err != nil {
				serverErrChan <- err
			}
		}()

		// 等待服务器启动
		err := utils.WaitConnect(hostPort, 5)
		require.NoError(t, err, "failed to wait for server to start")

		log.Infof("SSE MCP server started successfully on %s", sseURL)

		// 确保测试结束时关闭服务器
		defer func() {
			log.Infof("Shutting down SSE MCP server")
			// 使用一个新的 context，因为原来的可能已经过期
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			
			// 在 goroutine 中关闭，避免阻塞太久
			done := make(chan struct{})
			go func() {
				defer close(done)
				_ = sseServer.Shutdown(shutdownCtx)
			}()
			
			// 等待关闭完成或超时
			select {
			case <-done:
				log.Info("SSE MCP server shutdown completed")
			case <-time.After(3 * time.Second):
				log.Warn("SSE MCP server shutdown timed out")
			}
		}()

		// Step 2: 保存 MCP Server 配置到数据库
		t.Run("SaveMCPServerToDB", func(t *testing.T) {
			db := consts.GetGormProfileDatabase()
			require.NotNil(t, db, "profile database is nil")

			// 创建 MCP 服务器配置（使用 SSE 类型）
			mcpServerConfig := &schema.MCPServer{
				Name:   serverName,
				Type:   "sse",
				URL:    sseURL,
				Enable: true,
			}

			// 保存到数据库
			err := yakit.CreateMCPServer(db, mcpServerConfig)
			require.NoError(t, err, "failed to create mcp server in database")

			// 验证保存成功
			var savedServer schema.MCPServer
			err = db.Where("name = ?", serverName).First(&savedServer).Error
			require.NoError(t, err, "failed to query saved mcp server")
			assert.Equal(t, serverName, savedServer.Name)
			assert.Equal(t, "sse", savedServer.Type)
			assert.Equal(t, sseURL, savedServer.URL)
			assert.True(t, savedServer.Enable)

			log.Infof("MCP Server config saved to database: %s", serverName)
		})

		// Step 3: 使用 LoadAIToolFromMCPServers 加载工具
		t.Run("LoadAITools", func(t *testing.T) {
			// 等待一下确保数据库写入完成
			time.Sleep(500 * time.Millisecond)

			// 加载 AITool
			aiTools, err := LoadAIToolFromMCPServers(serverName, nil)
			require.NoError(t, err, "failed to load ai tools from mcp server")
			require.NotEmpty(t, aiTools, "no ai tools loaded")

			log.Infof("Loaded %d AI tools from MCP server", len(aiTools))

			// Step 4: 验证生成的 AITool 结构完整性
			t.Run("ValidateAIToolStructure", func(t *testing.T) {
				for _, tool := range aiTools {
					// 验证工具名称格式: mcp_{server_name}_{tool_name}
					assert.Contains(t, tool.Name, "mcp_"+serverName+"_",
						"tool name should have correct prefix")

					// 验证工具描述
					assert.NotEmpty(t, tool.Description, "tool description should not be empty")
					assert.Contains(t, tool.Description, "[MCP:"+serverName+"]",
						"tool description should contain server name")

					// 验证工具有回调函数
					assert.NotNil(t, tool.Callback, "tool callback should not be nil")

					// 验证工具的 InputSchema
					assert.NotNil(t, tool.InputSchema.Properties,
						"tool input schema properties should not be nil")

					log.Infof("Tool validated: %s", tool.Name)
					log.Infof("  Description: %s", tool.Description)
					log.Infof("  Parameters: %d", tool.InputSchema.Properties.Len())

					// 测试调用第一个工具
					if tool == aiTools[0] {
						t.Run("CallFirstTool", func(t *testing.T) {
							ctx := context.Background()
							// 为 test_echo 工具提供 message 参数
							params := InvokeParams{
								"message": "Hello, MCP!",
							}

							// 创建 stdout 和 stderr 缓冲区
							stdout := &testWriter{}
							stderr := &testWriter{}

							// 调用工具
							result, err := tool.Callback(ctx, params, nil, stdout, stderr)

							// 验证调用结果
							assert.NoError(t, err, "tool callback should not return error")
							assert.NotNil(t, result, "tool result should not be nil")

							log.Infof("Tool call result: %v", result)
							log.Infof("Tool stdout: %s", stdout.String())
							log.Infof("Tool stderr: %s", stderr.String())

							// 验证返回内容包含 Echo 消息
							resultStr, ok := result.(string)
							assert.True(t, ok, "result should be a string")
							assert.Contains(t, resultStr, "Echo: Hello, MCP!", "result should contain the echo message")
						})
					}
				}
			})
		})

	})
}

// testWriter 是一个简单的 io.Writer 实现，用于测试
type testWriter struct {
	data []byte
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *testWriter) String() string {
	return string(w.data)
}

// TestLoadAIToolFromMCPServers_NotFound 测试服务器不存在的情况
func TestLoadAIToolFromMCPServers_NotFound(t *testing.T) {
	_, err := LoadAIToolFromMCPServers("non_existent_server", nil)
	require.Error(t, err, "should return error for non-existent server")
	assert.Contains(t, err.Error(), "not found", "error should indicate server not found")
}

// TestLoadAIToolFromMCPServers_Disabled 测试服务器被禁用的情况
func TestLoadAIToolFromMCPServers_Disabled(t *testing.T) {
	serverName := "test_disabled_server"

	// 清理函数
	defer func() {
		db := consts.GetGormProfileDatabase()
		if db != nil {
			var server schema.MCPServer
			if err := db.Where("name = ?", serverName).First(&server).Error; err == nil {
				db.Unscoped().Delete(&server)
			}
		}
	}()

	// 创建一个禁用的服务器配置
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db)

	mcpServerConfig := &schema.MCPServer{
		Name:   serverName,
		Type:   "sse",
		URL:    "http://localhost:9999/sse",
		Enable: false, // 禁用
	}

	err := yakit.CreateMCPServer(db, mcpServerConfig)
	require.NoError(t, err)

	// 尝试加载工具
	_, err = LoadAIToolFromMCPServers(serverName, nil)
	require.Error(t, err, "should return error for disabled server")
	assert.Contains(t, err.Error(), "not found", "error should indicate server not found")
}

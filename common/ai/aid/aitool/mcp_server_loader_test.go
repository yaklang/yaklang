package aitool

import (
	"context"
	"fmt"
	"net/http"
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
		serverStarted := make(chan struct{})
		go func() {
			close(serverStarted) // 标记服务器 goroutine 已启动
			if err := sseServer.Start(hostPort); err != nil && err != http.ErrServerClosed {
				log.Errorf("SSE server error: %v", err)
			}
		}()

		// 等待服务器 goroutine 启动并等待服务器就绪
		<-serverStarted
		time.Sleep(50 * time.Millisecond) // 等待一小段时间确保 server 对象被初始化
		err := utils.WaitConnect(hostPort, 5)
		require.NoError(t, err, "failed to wait for server to start")

		log.Infof("SSE MCP server started successfully on %s", sseURL)

		// 注意：我们不显式调用 Shutdown，因为：
		// 1. MCP 客户端连接可能仍然打开，导致 Shutdown 超时
		// 2. 测试进程结束时会自动清理所有资源（socket、goroutine 等）
		// 3. 在生产环境中，应该先关闭所有 MCP 客户端，然后再关闭服务器

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
			aiTools, err := LoadAIToolFromMCPServers(nil, context.Background(), serverName)
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
	_, err := LoadAIToolFromMCPServers(nil, context.Background(), "non_existent_server")
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
	_, err = LoadAIToolFromMCPServers(nil, context.Background(), serverName)
	require.Error(t, err, "should return error for disabled server")
	assert.Contains(t, err.Error(), "not found", "error should indicate server not found")
}

// TestLoadAllEnabledAIToolsFromMCPServers 测试加载所有启用的 MCP 服务器的工具
func TestLoadAllEnabledAIToolsFromMCPServers(t *testing.T) {
	// 测试服务器名称
	serverName1 := "test_all_enabled_server_1"
	serverName2 := "test_all_enabled_server_2"
	serverName3 := "test_all_enabled_server_disabled"

	// 测试初始化：清理可能存在的旧数据
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db, "profile database is nil")

	// 清理函数
	defer func() {
		db := consts.GetGormProfileDatabase()
		if db != nil {
			for _, name := range []string{serverName1, serverName2, serverName3} {
				var server schema.MCPServer
				if err := db.Where("name = ?", name).First(&server).Error; err == nil {
					db.Unscoped().Delete(&server)
					log.Infof("cleaned up test mcp server: %s", name)
				}
			}
		}
	}()

	// 启动两个 SSE MCP 服务器
	var sseURL1, sseURL2 string

	// 启动第一个服务器
	t.Run("StartServer1", func(t *testing.T) {
		mcpServer := server.NewMCPServer("Test MCP Server 1", "1.0.0")

		// 添加测试工具
		testTool := mcp.NewTool(
			"test_tool_1",
			mcp.WithDescription("Test tool from server 1"),
			mcp.WithString("input", mcp.Description("Input parameter"), mcp.Required()),
		)

		mcpServer.AddTool(testTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []any{mcp.TextContent{Type: "text", Text: "Response from server 1"}},
				IsError: false,
			}, nil
		})

		port := utils.GetRandomAvailableTCPPort()
		host := "127.0.0.1"
		hostPort := utils.HostPort(host, port)
		baseURL := fmt.Sprintf("http://%s", hostPort)
		sseURL1 = baseURL + "/sse"

		sseServer := server.NewSSEServer(mcpServer, baseURL)

		serverStarted := make(chan struct{})
		go func() {
			close(serverStarted)
			if err := sseServer.Start(hostPort); err != nil && err != http.ErrServerClosed {
				log.Errorf("SSE server 1 error: %v", err)
			}
		}()

		<-serverStarted
		time.Sleep(50 * time.Millisecond)
		err := utils.WaitConnect(hostPort, 5)
		require.NoError(t, err, "failed to wait for server 1 to start")

		log.Infof("SSE MCP server 1 started on %s", sseURL1)
	})

	// 启动第二个服务器
	t.Run("StartServer2", func(t *testing.T) {
		mcpServer := server.NewMCPServer("Test MCP Server 2", "1.0.0")

		// 添加测试工具
		testTool := mcp.NewTool(
			"test_tool_2",
			mcp.WithDescription("Test tool from server 2"),
			mcp.WithString("input", mcp.Description("Input parameter"), mcp.Required()),
		)

		mcpServer.AddTool(testTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []any{mcp.TextContent{Type: "text", Text: "Response from server 2"}},
				IsError: false,
			}, nil
		})

		port := utils.GetRandomAvailableTCPPort()
		host := "127.0.0.1"
		hostPort := utils.HostPort(host, port)
		baseURL := fmt.Sprintf("http://%s", hostPort)
		sseURL2 = baseURL + "/sse"

		sseServer := server.NewSSEServer(mcpServer, baseURL)

		serverStarted := make(chan struct{})
		go func() {
			close(serverStarted)
			if err := sseServer.Start(hostPort); err != nil && err != http.ErrServerClosed {
				log.Errorf("SSE server 2 error: %v", err)
			}
		}()

		<-serverStarted
		time.Sleep(50 * time.Millisecond)
		err := utils.WaitConnect(hostPort, 5)
		require.NoError(t, err, "failed to wait for server 2 to start")

		log.Infof("SSE MCP server 2 started on %s", sseURL2)
	})

	// 保存服务器配置到数据库
	t.Run("SaveServersToDatabase", func(t *testing.T) {
		// 保存第一个启用的服务器
		mcpServerConfig1 := &schema.MCPServer{
			Name:   serverName1,
			Type:   "sse",
			URL:    sseURL1,
			Enable: true,
		}
		err := yakit.CreateMCPServer(db, mcpServerConfig1)
		require.NoError(t, err, "failed to create mcp server 1")

		// 保存第二个启用的服务器
		mcpServerConfig2 := &schema.MCPServer{
			Name:   serverName2,
			Type:   "sse",
			URL:    sseURL2,
			Enable: true,
		}
		err = yakit.CreateMCPServer(db, mcpServerConfig2)
		require.NoError(t, err, "failed to create mcp server 2")

		// 保存一个禁用的服务器
		mcpServerConfig3 := &schema.MCPServer{
			Name:   serverName3,
			Type:   "sse",
			URL:    "http://localhost:9999/sse",
			Enable: false,
		}
		err = yakit.CreateMCPServer(db, mcpServerConfig3)
		require.NoError(t, err, "failed to create mcp server 3")

		log.Infof("All test MCP servers saved to database")
	})

	// 测试加载所有启用的服务器的工具
	t.Run("LoadAllEnabledTools", func(t *testing.T) {
		// 等待数据库写入完成
		time.Sleep(500 * time.Millisecond)

		ctx := context.Background()
		aiTools, err := LoadAllEnabledAIToolsFromMCPServers(db, ctx)
		require.NoError(t, err, "failed to load all enabled ai tools")
		require.NotEmpty(t, aiTools, "no ai tools loaded")

		log.Infof("Loaded %d AI tools from all enabled MCP servers", len(aiTools))

		// 验证加载的工具数量（应该是2个，因为有2个启用的服务器，每个有1个工具）
		assert.GreaterOrEqual(t, len(aiTools), 2, "should load at least 2 tools from 2 enabled servers")

		// 验证工具来自不同的服务器
		serverNames := make(map[string]bool)
		for _, tool := range aiTools {
			// 工具名称格式: mcp_{server_name}_{tool_name}
			if utils.MatchAnyOfSubString(tool.Name, serverName1) {
				serverNames[serverName1] = true
			}
			if utils.MatchAnyOfSubString(tool.Name, serverName2) {
				serverNames[serverName2] = true
			}
			// 不应该包含禁用的服务器
			assert.False(t, utils.MatchAnyOfSubString(tool.Name, serverName3),
				"should not load tools from disabled server")

			log.Infof("Loaded tool: %s", tool.Name)
		}

		// 验证至少从两个不同的服务器加载了工具
		assert.True(t, serverNames[serverName1], "should load tools from server 1")
		assert.True(t, serverNames[serverName2], "should load tools from server 2")
	})
}

// TestLoadAllEnabledAIToolsFromMCPServers_Empty 测试没有启用的服务器时的情况
func TestLoadAllEnabledAIToolsFromMCPServers_Empty(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db)

	// 创建一个临时的禁用服务器
	serverName := "test_empty_disabled_server"
	defer func() {
		var server schema.MCPServer
		if err := db.Where("name = ?", serverName).First(&server).Error; err == nil {
			db.Unscoped().Delete(&server)
		}
	}()

	mcpServerConfig := &schema.MCPServer{
		Name:   serverName,
		Type:   "sse",
		URL:    "http://localhost:9999/sse",
		Enable: false,
	}
	err := yakit.CreateMCPServer(db, mcpServerConfig)
	require.NoError(t, err)

	// 加载所有启用的工具（应该不包含这个禁用的服务器）
	ctx := context.Background()
	aiTools, err := LoadAllEnabledAIToolsFromMCPServers(db, ctx)

	// 不应该报错，只是返回空列表或不包含禁用服务器的工具
	require.NoError(t, err)

	// 验证不包含禁用服务器的工具
	for _, tool := range aiTools {
		assert.False(t, utils.MatchAnyOfSubString(tool.Name, serverName),
			"should not load tools from disabled server")
	}
}

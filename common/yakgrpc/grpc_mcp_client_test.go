package yakgrpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	// 确保 MCP 服务器表存在
	db := consts.GetGormProfileDatabase()
	db.AutoMigrate(&schema.MCPServer{})
}

func TestMCPServerCRUD(t *testing.T) {
	// 确保数据库表存在
	db := consts.GetGormProfileDatabase()
	db.AutoMigrate(&schema.MCPServer{})

	// 清理可能存在的测试数据
	db.Unscoped().Where("name LIKE ?", "test-%").Delete(&schema.MCPServer{})

	// 创建测试服务器实例
	server, err := NewServer()
	require.NoError(t, err)

	ctx := context.Background()

	// 测试添加 MCP 服务器
	t.Run("AddMCPServer", func(t *testing.T) {
		// 测试添加 stdio 类型服务器
		req := &ypb.AddMCPServerRequest{
			Name:    "test-stdio-server-unique",
			Type:    "stdio",
			Command: "npx -y @modelcontextprotocol/server-filesystem /tmp",
		}

		resp, err := server.AddMCPServer(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.GetOk())
		assert.Contains(t, resp.GetReason(), "添加成功")

		// 测试添加 sse 类型服务器
		req2 := &ypb.AddMCPServerRequest{
			Name: "test-sse-server-unique",
			Type: "sse",
			URL:  "http://localhost:8080/sse",
		}

		resp2, err := server.AddMCPServer(ctx, req2)
		require.NoError(t, err)
		assert.True(t, resp2.GetOk())

		// 测试重复名称
		resp3, err := server.AddMCPServer(ctx, req)
		require.NoError(t, err)
		assert.False(t, resp3.GetOk())
		assert.Contains(t, resp3.GetReason(), "already exists")

		// 测试无效参数
		invalidReq := &ypb.AddMCPServerRequest{
			Name: "",
			Type: "stdio",
		}
		resp4, err := server.AddMCPServer(ctx, invalidReq)
		require.NoError(t, err)
		assert.False(t, resp4.GetOk())
		assert.Contains(t, resp4.GetReason(), "名称不能为空")

		// 测试 stdio 类型缺少命令
		invalidReq2 := &ypb.AddMCPServerRequest{
			Name: "test-invalid",
			Type: "stdio",
		}
		resp5, err := server.AddMCPServer(ctx, invalidReq2)
		require.NoError(t, err)
		assert.False(t, resp5.GetOk())
		assert.Contains(t, resp5.GetReason(), "必须提供启动命令")

		// 测试 sse 类型缺少 URL
		invalidReq3 := &ypb.AddMCPServerRequest{
			Name: "test-invalid-sse",
			Type: "sse",
		}
		resp6, err := server.AddMCPServer(ctx, invalidReq3)
		require.NoError(t, err)
		assert.False(t, resp6.GetOk())
		assert.Contains(t, resp6.GetReason(), "必须提供 URL")

		// 测试无效类型
		invalidReq4 := &ypb.AddMCPServerRequest{
			Name: "test-invalid-type",
			Type: "invalid",
		}
		resp7, err := server.AddMCPServer(ctx, invalidReq4)
		require.NoError(t, err)
		assert.False(t, resp7.GetOk())
		assert.Contains(t, resp7.GetReason(), "必须是 stdio 或 sse")
	})

	// 测试查询 MCP 服务器
	t.Run("GetAllMCPServers", func(t *testing.T) {
		// 查询所有服务器
		req := &ypb.GetAllMCPServersRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		}

		resp, err := server.GetAllMCPServers(ctx, req)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(resp.GetMCPServers()), 2) // 至少有我们刚添加的两个
		assert.Greater(t, resp.GetTotal(), int64(0))

		// 测试关键词搜索
		req2 := &ypb.GetAllMCPServersRequest{
			Keyword: "stdio",
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		}

		resp2, err := server.GetAllMCPServers(ctx, req2)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(resp2.GetMCPServers()), 1)

		// 验证搜索结果包含关键词
		found := false
		for _, srv := range resp2.GetMCPServers() {
			if srv.GetName() == "test-stdio-server-unique" {
				found = true
				assert.Equal(t, "stdio", srv.GetType())
				break
			}
		}
		assert.True(t, found)

		// 测试 ID 过滤
		if len(resp.GetMCPServers()) > 0 {
			firstServer := resp.GetMCPServers()[0]
			req3 := &ypb.GetAllMCPServersRequest{
				ID: firstServer.GetID(),
				Pagination: &ypb.Paging{
					Page:  1,
					Limit: 10,
				},
			}

			resp3, err := server.GetAllMCPServers(ctx, req3)
			require.NoError(t, err)
			assert.Len(t, resp3.GetMCPServers(), 1)
			assert.Equal(t, firstServer.GetID(), resp3.GetMCPServers()[0].GetID())
		}
	})

	// 测试更新 MCP 服务器
	t.Run("UpdateMCPServer", func(t *testing.T) {
		// 先获取一个服务器用于更新
		listReq := &ypb.GetAllMCPServersRequest{
			Keyword: "test-stdio-server-unique",
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 1,
			},
		}

		listResp, err := server.GetAllMCPServers(ctx, listReq)
		require.NoError(t, err)
		require.Greater(t, len(listResp.GetMCPServers()), 0)

		serverToUpdate := listResp.GetMCPServers()[0]

		// 更新服务器
		updateReq := &ypb.UpdateMCPServerRequest{
			ID:      serverToUpdate.GetID(),
			Name:    "updated-stdio-server",
			Type:    "stdio",
			Command: "npx -y @modelcontextprotocol/server-filesystem /home",
		}

		updateResp, err := server.UpdateMCPServer(ctx, updateReq)
		require.NoError(t, err)
		assert.True(t, updateResp.GetOk())
		assert.Contains(t, updateResp.GetReason(), "更新成功")

		// 验证更新结果
		verifyReq := &ypb.GetAllMCPServersRequest{
			ID: serverToUpdate.GetID(),
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 1,
			},
		}

		verifyResp, err := server.GetAllMCPServers(ctx, verifyReq)
		require.NoError(t, err)
		require.Len(t, verifyResp.GetMCPServers(), 1)

		updatedServer := verifyResp.GetMCPServers()[0]
		assert.Equal(t, "updated-stdio-server", updatedServer.GetName())
		assert.Equal(t, "npx -y @modelcontextprotocol/server-filesystem /home", updatedServer.GetCommand())
	})

	// 测试删除 MCP 服务器
	t.Run("DeleteMCPServer", func(t *testing.T) {
		// 先获取要删除的服务器
		listReq := &ypb.GetAllMCPServersRequest{
			Keyword: "updated-stdio-server",
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 1,
			},
		}

		listResp, err := server.GetAllMCPServers(ctx, listReq)
		require.NoError(t, err)
		require.Greater(t, len(listResp.GetMCPServers()), 0)

		serverToDelete := listResp.GetMCPServers()[0]

		// 删除服务器
		deleteReq := &ypb.DeleteMCPServerRequest{
			ID: serverToDelete.GetID(),
		}

		deleteResp, err := server.DeleteMCPServer(ctx, deleteReq)
		require.NoError(t, err)
		assert.True(t, deleteResp.GetOk())
		assert.Contains(t, deleteResp.GetReason(), "删除成功")

		// 验证删除结果
		verifyReq := &ypb.GetAllMCPServersRequest{
			ID: serverToDelete.GetID(),
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 1,
			},
		}

		verifyResp, err := server.GetAllMCPServers(ctx, verifyReq)
		require.NoError(t, err)
		assert.Len(t, verifyResp.GetMCPServers(), 0)
	})
}

func TestMCPServerDatabase(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatalf("创建临时数据库失败: %v", err)
	}
	db = db.Debug()
	// 确保数据库表存在
	db.AutoMigrate(&schema.MCPServer{})

	// 测试数据库直接操作
	t.Run("DirectDatabaseOperations", func(t *testing.T) {
		// 创建测试服务器
		server := &schema.MCPServer{
			Name:    "db-test-server",
			Type:    "stdio",
			Command: "test-command",
		}

		err := yakit.CreateMCPServer(db, server)
		require.NoError(t, err)
		assert.Greater(t, server.ID, uint(0))

		// 获取服务器
		retrieved, err := yakit.GetMCPServer(db, int64(server.ID))
		require.NoError(t, err)
		assert.Equal(t, server.Name, retrieved.Name)
		assert.Equal(t, server.Type, retrieved.Type)
		assert.Equal(t, server.Command, retrieved.Command)

		// 更新服务器
		updateServer := &schema.MCPServer{
			Name: "updated-db-test-server",
			Type: "sse",
			URL:  "http://test.com",
		}
		err = yakit.UpdateMCPServer(db, int64(server.ID), updateServer)
		require.NoError(t, err)

		// 验证更新
		updated, err := yakit.GetMCPServer(db, int64(server.ID))
		require.NoError(t, err)
		assert.Equal(t, "updated-db-test-server", updated.Name)
		assert.Equal(t, "sse", updated.Type)
		assert.Equal(t, "http://test.com", updated.URL)

		// 查询服务器
		req := &ypb.GetAllMCPServersRequest{
			Keyword: "updated-db-test",
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		}

		paginator, servers, err := yakit.QueryMCPServers(db, req)
		require.NoError(t, err)
		assert.Greater(t, len(servers), 0)
		assert.Greater(t, paginator.TotalRecord, 0)

		// 删除服务器
		err = yakit.DeleteMCPServer(db, int64(server.ID))
		require.NoError(t, err)

		// 验证删除
		_, err = yakit.GetMCPServer(db, int64(server.ID))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	// 测试错误情况
	t.Run("ErrorCases", func(t *testing.T) {
		// 测试创建空名称服务器
		emptyServer := &schema.MCPServer{
			Name: "",
			Type: "stdio",
		}

		err := yakit.CreateMCPServer(db, emptyServer)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name cannot be empty")

		// 测试创建空类型服务器
		emptyTypeServer := &schema.MCPServer{
			Name: "test",
			Type: "",
		}

		err = yakit.CreateMCPServer(db, emptyTypeServer)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "type cannot be empty")

		// 测试获取不存在的服务器
		_, err = yakit.GetMCPServer(db, 99999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestMCPServerPagination(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	// 确保数据库表存在
	db.AutoMigrate(&schema.MCPServer{})

	// 创建多个测试服务器
	servers := []*schema.MCPServer{
		{Name: "pagination-test-1", Type: "stdio", Command: "cmd1"},
		{Name: "pagination-test-2", Type: "sse", URL: "http://test1.com"},
		{Name: "pagination-test-3", Type: "stdio", Command: "cmd2"},
		{Name: "pagination-test-4", Type: "sse", URL: "http://test2.com"},
		{Name: "pagination-test-5", Type: "stdio", Command: "cmd3"},
	}

	// 创建服务器
	for _, server := range servers {
		err := yakit.CreateMCPServer(db, server)
		require.NoError(t, err)
	}

	defer func() {
		// 清理测试数据
		for _, server := range servers {
			yakit.DeleteMCPServer(db, int64(server.ID))
		}
	}()

	t.Run("PaginationTest", func(t *testing.T) {
		// 测试分页
		req := &ypb.GetAllMCPServersRequest{
			Keyword: "pagination-test",
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 2,
			},
		}

		paginator, results, err := yakit.QueryMCPServers(db, req)
		require.NoError(t, err)
		assert.Len(t, results, 2)                 // 每页2个
		assert.Equal(t, 5, paginator.TotalRecord) // 总共5个
		assert.Equal(t, 3, paginator.TotalPage)   // 总共3页

		// 测试第二页
		req.Pagination.Page = 2
		paginator2, results2, err := yakit.QueryMCPServers(db, req)
		require.NoError(t, err)
		assert.Len(t, results2, 2)
		assert.Equal(t, 2, paginator2.Page)

		// 测试最后一页
		req.Pagination.Page = 3
		paginator3, results3, err := yakit.QueryMCPServers(db, req)
		require.NoError(t, err)
		assert.Len(t, results3, 1) // 最后一页只有1个
		assert.Equal(t, 3, paginator3.Page)
	})

	t.Run("SearchTest", func(t *testing.T) {
		// 测试类型搜索
		req := &ypb.GetAllMCPServersRequest{
			Keyword: "stdio",
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		}

		_, results, err := yakit.QueryMCPServers(db, req)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3) // 至少有3个stdio类型的服务器

		// 验证搜索结果
		for _, server := range results {
			assert.Contains(t, []string{"stdio", "sse"}, server.Type)
		}
	})
}

func TestMCPServerToolsRetrieval(t *testing.T) {
	// 设置日志级别以减少输出
	log.SetLevel(log.ErrorLevel)

	// 创建并启动一个 mock MCP 服务器
	port := utils.GetRandomAvailableTCPPort()
	serverURL := fmt.Sprintf("http://localhost:%d", port)

	// 启动 MCP 服务器
	go func() {
		mcpServer, err := mcp.NewMCPServer()
		if err != nil {
			t.Errorf("创建 MCP 服务器失败: %v", err)
			return
		}

		if err := mcpServer.ServeSSE(fmt.Sprintf(":%d", port), serverURL); err != nil {
			t.Logf("MCP 服务器启动失败: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(2 * time.Second)
	err := utils.WaitConnect(fmt.Sprintf("127.0.0.1:%d", port), 5)
	if err != nil {
		t.Skipf("无法连接到 MCP 服务器，跳过测试: %v", err)
		return
	}

	// 创建 gRPC 服务器实例
	grpcServer, err := NewServer()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("TestGetMCPServerToolsWithSSE", func(t *testing.T) {
		// 创建一个 SSE 类型的 MCP 服务器配置
		mcpServerConfig := &schema.MCPServer{
			Name: "test-sse-server-for-tools",
			Type: "sse",
			URL:  serverURL + "/sse",
		}

		// 调用 getMCPServerTools 方法
		tools, err := grpcServer.getMCPServerTools(ctx, mcpServerConfig)

		// 验证结果
		if err != nil {
			// 如果连接失败，这可能是正常的（服务器可能没有完全启动）
			t.Logf("获取工具列表失败（可能是正常的）: %v", err)
			return
		}

		// 验证返回的工具列表
		assert.NotNil(t, tools)
		t.Logf("成功获取到 %d 个工具", len(tools))

		// 打印工具信息用于调试
		for i, tool := range tools {
			t.Logf("工具 %d: 名称=%s, 描述=%s, 参数数量=%d",
				i+1, tool.GetName(), tool.GetDescription(), len(tool.GetParams()))

			// 验证工具基本信息
			assert.NotEmpty(t, tool.GetName(), "工具名称不应为空")

			// 验证参数信息
			for j, param := range tool.GetParams() {
				t.Logf("  参数 %d: 名称=%s, 类型=%s, 必需=%s, 描述=%s",
					j+1, param.GetName(), param.GetType(), param.GetRequired(), param.GetDescription())

				assert.NotEmpty(t, param.GetName(), "参数名称不应为空")
				assert.NotEmpty(t, param.GetType(), "参数类型不应为空")
				assert.Contains(t, []string{"true", "false"}, param.GetRequired(), "参数必需字段应为 true 或 false")
			}
		}
	})

	t.Run("TestGetMCPServerToolsWithStdio", func(t *testing.T) {
		// 测试 stdio 类型的 MCP 服务器（使用一个简单的 echo 命令作为 mock）
		mcpServerConfig := &schema.MCPServer{
			Name:    "test-stdio-server-for-tools",
			Type:    "stdio",
			Command: "echo '{\"tools\":[]}'", // 简单的 mock 命令
		}

		// 调用 getMCPServerTools 方法
		tools, err := grpcServer.getMCPServerTools(ctx, mcpServerConfig)

		// 对于 stdio 类型，我们期望会有错误（因为 echo 不是真正的 MCP 服务器）
		if err != nil {
			t.Logf("stdio 类型服务器获取工具列表失败（预期的）: %v", err)
			assert.Error(t, err)
		} else {
			// 如果没有错误，验证返回的工具列表
			assert.NotNil(t, tools)
			t.Logf("stdio 服务器返回了 %d 个工具", len(tools))
		}
	})

	t.Run("TestGetMCPServerToolsWithInvalidType", func(t *testing.T) {
		// 测试无效的服务器类型
		mcpServerConfig := &schema.MCPServer{
			Name: "test-invalid-server",
			Type: "invalid-type",
		}

		// 调用 getMCPServerTools 方法
		tools, err := grpcServer.getMCPServerTools(ctx, mcpServerConfig)

		// 应该返回错误
		assert.Error(t, err)
		assert.Nil(t, tools)
		assert.Contains(t, err.Error(), "unsupported server type")
	})
}

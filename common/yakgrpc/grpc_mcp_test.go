package yakgrpc

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp"
	mcpclient "github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	rawmcp "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 测试前设置
func init() {
	mcp.RegisterNewLocalClient(func(locals ...bool) (mcp.YakClientInterface, error) {
		client, err := NewLocalClient(locals...)
		if err != nil {
			return nil, err
		}
		v, ok := client.(mcp.YakClientInterface)
		if !ok {
			return nil, utils.Error("failed to cast client to yakgrpc.Client")
		}
		return v, nil
	})
}

func TestGRPC_StartMcpServer_BasicFlow(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 创建启动请求
	req := &ypb.StartMcpServerRequest{
		Host:      "127.0.0.1",
		Port:      0, // 使用随机端口
		EnableAll: true,
	}

	stream, err := client.StartMcpServer(ctx, req)
	require.NoError(t, err)

	var responses []*ypb.StartMcpServerResponse
	var serverUrl string

	// 接收前几个状态消息
	for i := 0; i < 3; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.NotNil(t, resp)
		responses = append(responses, resp)

		if resp.ServerUrl != "" {
			serverUrl = resp.ServerUrl
		}

		log.Infof("Received MCP response: Status=%s, Message=%s, ServerUrl=%s",
			resp.Status, resp.Message, resp.ServerUrl)

		if resp.Status == "running" {
			break
		}
	}

	// 验证收到的响应
	require.GreaterOrEqual(t, len(responses), 2)

	// 第一个响应应该是 starting 状态
	require.Equal(t, "starting", responses[0].Status)
	require.Contains(t, responses[0].Message, "Initializing MCP server")

	// 应该有一个 configured 状态
	configuredFound := false
	runningFound := false
	for _, resp := range responses {
		if resp.Status == "configured" {
			configuredFound = true
		}
		if resp.Status == "running" {
			runningFound = true
			require.NotEmpty(t, resp.ServerUrl, "ServerUrl should be set when status is running")
		}
	}
	require.True(t, configuredFound, "Should receive configured status")
	require.True(t, runningFound, "Should receive running status")
	require.NotEmpty(t, serverUrl, "ServerUrl should be provided")

	// 验证 URL 格式
	require.Contains(t, serverUrl, "http://127.0.0.1:", "ServerUrl should contain correct host")

	// 创建 SSE MCP 客户端
	mcpClient, err := mcpclient.NewSSEMCPClient(serverUrl)
	if err != nil {
		t.Fatalf("创建 MCP 客户端失败: %v", err)
	}
	defer mcpClient.Close()

	// 设置上下文和超时
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 启动客户端连接
	err = mcpClient.Start(ctx)
	if err != nil {
		t.Fatalf("启动 MCP 客户端连接失败: %v", err)
	}

	// 初始化客户端
	t.Log("初始化 MCP 客户端...")
	initRequest := rawmcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = rawmcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = rawmcp.Implementation{
		Name:    "dump-mcp-tools-client",
		Version: "1.0.0",
	}

	initResult, err := mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	t.Logf("初始化成功，服务器名称: %s, 版本: %s",
		initResult.ServerInfo.Name,
		initResult.ServerInfo.Version)

	toolsRequest := rawmcp.ListToolsRequest{}
	toolsResult, err := mcpClient.ListTools(ctx, toolsRequest)
	assert.NoError(t, err)
	assert.NotNil(t, toolsResult)
	assert.True(t, len(toolsResult.Tools) > 0)
	log.Infof("获取到 %d 个工具", len(toolsResult.Tools))
}

func TestGRPC_StartMcpServer_DefaultPort(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 不指定端口，应该使用随机端口
	req := &ypb.StartMcpServerRequest{
		// 不设置 Host 和 Port，使用默认值
		Tool: []string{"codec"},
	}

	stream, err := client.StartMcpServer(ctx, req)
	require.NoError(t, err)

	// 接收前几个状态消息
	for i := 0; i < 3; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if resp.Status == "running" {
			require.NotEmpty(t, resp.ServerUrl)
			require.Contains(t, resp.ServerUrl, "http://127.0.0.1:", "Should use default host")
			break
		}
	}
}

func TestGRPC_StartMcpServer_WithAITool(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	// 第一步：创建一个 AI Tool
	toolName := "test_mcp_tool_" + utils.RandStringBytes(8)
	toolDescription := "Test tool for MCP server validation"
	toolContent := `
cli.String("url", cli.setRequired(true))
`

	saveReq := &ypb.SaveAIToolRequest{
		Name:        toolName,
		Description: toolDescription,
		Content:     toolContent,
		Keywords:    []string{"test", "mcp"},
	}

	saveResp, err := client.SaveAIToolV2(context.Background(), saveReq)
	require.NoError(t, err)
	require.NotNil(t, saveResp)
	require.True(t, saveResp.IsSuccess, "Tool should be created successfully")

	// 清理：测试结束后删除创建的工具
	defer func() {
		deleteReq := &ypb.DeleteAIToolRequest{
			ToolNames: []string{toolName},
		}
		_, _ = client.DeleteAITool(context.Background(), deleteReq)
	}()

	// 第二步：启动 MCP Server
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	startReq := &ypb.StartMcpServerRequest{
		Host:                  "127.0.0.1",
		Port:                  0,
		EnableAll:             true,
		EnableAIToolFramework: true,
	}

	stream, err := client.StartMcpServer(ctx, startReq)
	require.NoError(t, err)

	var serverUrl string

	// 接收状态消息，直到服务器启动
	for i := 0; i < 5; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.NotNil(t, resp)

		if resp.ServerUrl != "" {
			serverUrl = resp.ServerUrl
		}

		log.Infof("MCP Server Status: %s, Message: %s", resp.Status, resp.Message)

		if resp.Status == "running" {
			break
		}
	}

	require.NotEmpty(t, serverUrl, "Server URL should be provided")

	// 第三步：连接 MCP 客户端并验证工具是否存在
	mcpClient, err := mcpclient.NewSSEMCPClient(serverUrl)
	require.NoError(t, err)
	defer mcpClient.Close()

	// 启动客户端连接
	clientCtx, clientCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer clientCancel()

	err = mcpClient.Start(clientCtx)
	require.NoError(t, err)

	// 初始化客户端
	initRequest := rawmcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = rawmcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = rawmcp.Implementation{
		Name:    "test-mcp-aitool-client",
		Version: "1.0.0",
	}

	initResult, err := mcpClient.Initialize(clientCtx, initRequest)
	require.NoError(t, err)
	require.NotNil(t, initResult)
	log.Infof("MCP Client initialized with server: %s %s",
		initResult.ServerInfo.Name,
		initResult.ServerInfo.Version)

	// 第四步：列出所有工具并验证我们创建的工具是否存在
	toolsRequest := rawmcp.ListToolsRequest{}
	toolsResult, err := mcpClient.ListTools(clientCtx, toolsRequest)
	require.NoError(t, err)
	require.NotNil(t, toolsResult)
	require.Greater(t, len(toolsResult.Tools), 0, "Should have at least one tool")

	log.Infof("Total tools available: %d", len(toolsResult.Tools))

	// 查找我们创建的工具
	toolFound := false
	for _, tool := range toolsResult.Tools {
		log.Infof("Available tool: %s - %s", tool.Name, tool.Description)
		if tool.Name == toolName {
			toolFound = true
			assert.Equal(t, toolDescription, tool.Description, "Tool description should match")
			log.Infof("Found our test tool: %s", toolName)
			break
		}
	}

	// 验证工具必须存在
	require.True(t, toolFound, "Created AI tool '%s' should be available in MCP server", toolName)
}

func TestGRPC_StartMcpServer_ProjectDatabaseTools(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := &ypb.StartMcpServerRequest{
		Host: "127.0.0.1",
		Port: 0,
		Tool: []string{"project_database", "httpflow", "port_scan", "risk"},
	}

	stream, err := client.StartMcpServer(ctx, req)
	require.NoError(t, err)

	var serverURL string
	for i := 0; i < 5; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if resp.GetServerUrl() != "" {
			serverURL = resp.GetServerUrl()
		}
		if resp.GetStatus() == "running" {
			break
		}
	}
	require.NotEmpty(t, serverURL)

	mcpClient, err := mcpclient.NewSSEMCPClient(serverURL)
	require.NoError(t, err)
	defer mcpClient.Close()

	clientCtx, clientCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer clientCancel()

	require.NoError(t, mcpClient.Start(clientCtx))

	initRequest := rawmcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = rawmcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = rawmcp.Implementation{
		Name:    "test-mcp-project-database-client",
		Version: "1.0.0",
	}
	_, err = mcpClient.Initialize(clientCtx, initRequest)
	require.NoError(t, err)

	projectNameA := "mcp_project_db_a_" + utils.RandStringBytes(6)
	projectNameB := "mcp_project_db_b_" + utils.RandStringBytes(6)

	var createdProjectIDs []int64
	defer func() {
		for _, id := range createdProjectIDs {
			if id <= 0 {
				continue
			}
			_, _ = client.DeleteProject(context.Background(), &ypb.DeleteProjectRequest{
				Id:            id,
				IsDeleteLocal: true,
				Type:          yakit.TypeProject,
			})
		}
	}()

	createAReq := rawmcp.CallToolRequest{}
	createAReq.Params.Name = "create_project_database"
	createAReq.Params.Arguments = map[string]any{
		"projectName":     projectNameA,
		"description":     "created by grpc mcp test A",
		"switchToCurrent": true,
	}
	createAResult, err := mcpClient.CallTool(clientCtx, createAReq)
	require.NoError(t, err)
	createAData := mustExtractToolTextJSON(t, createAResult)
	idA := int64(createAData["created_project_id"].(float64))
	require.Greater(t, idA, int64(0))
	createdProjectIDs = append(createdProjectIDs, idA)
	require.Equal(t, true, createAData["switched_to_current"])

	createBReq := rawmcp.CallToolRequest{}
	createBReq.Params.Name = "create_project_database"
	createBReq.Params.Arguments = map[string]any{
		"projectName":     projectNameB,
		"description":     "created by grpc mcp test B",
		"switchToCurrent": false,
	}
	createBResult, err := mcpClient.CallTool(clientCtx, createBReq)
	require.NoError(t, err)
	createBData := mustExtractToolTextJSON(t, createBResult)
	idB := int64(createBData["created_project_id"].(float64))
	require.Greater(t, idB, int64(0))
	createdProjectIDs = append(createdProjectIDs, idB)
	require.Equal(t, false, createBData["switched_to_current"])

	listReq := rawmcp.CallToolRequest{}
	listReq.Params.Name = "list_project_databases"
	listReq.Params.Arguments = map[string]any{
		"keyword": "mcp_project_db_",
		"limit":   50,
	}
	listResult, err := mcpClient.CallTool(clientCtx, listReq)
	require.NoError(t, err)
	listItems := mustExtractToolJSONArray(t, listResult)
	require.NotEmpty(t, listItems)

	foundA := false
	foundB := false
	for _, item := range listItems {
		gotID := int64(item["id"].(float64))
		if gotID == idA {
			foundA = true
		}
		if gotID == idB {
			foundB = true
		}
	}
	require.True(t, foundA, "created project A should appear in list_project_databases")
	require.True(t, foundB, "created project B should appear in list_project_databases")

	switchReq := rawmcp.CallToolRequest{}
	switchReq.Params.Name = "switch_current_project_database"
	switchReq.Params.Arguments = map[string]any{
		"id": idB,
	}
	switchResult, err := mcpClient.CallTool(clientCtx, switchReq)
	require.NoError(t, err)
	switchData := mustExtractToolTextJSON(t, switchResult)
	currentProject := switchData["current_project"].(map[string]any)
	require.Equal(t, float64(idB), currentProject["Id"])

	_, err = client.SetCurrentProject(clientCtx, &ypb.SetCurrentProjectRequest{
		Id:   idA,
		Type: yakit.TypeProject,
	})
	require.NoError(t, err)

	urlA := "https://" + projectNameA + ".example"
	urlB := "https://" + projectNameB + ".example"
	hostA := projectNameA + ".local"
	hostB := projectNameB + ".local"
	riskTitleA := "risk-" + projectNameA
	riskTitleB := "risk-" + projectNameB
	projectA, ok := createAData["project"].(map[string]any)
	require.True(t, ok)
	projectAPath, ok := projectA["DatabasePath"].(string)
	require.True(t, ok)
	require.NotEmpty(t, projectAPath)
	projectADB, err := consts.CreateProjectDatabase(projectAPath)
	require.NoError(t, err)
	require.NoError(t, insertMCPProjectData(projectADB, urlA, hostA, riskTitleA))
	require.NoError(t, projectADB.Close())

	projectB, ok := createBData["project"].(map[string]any)
	require.True(t, ok)
	projectBPath, ok := projectB["DatabasePath"].(string)
	require.True(t, ok)
	require.NotEmpty(t, projectBPath)
	projectBDB, err := consts.CreateProjectDatabase(projectBPath)
	require.NoError(t, err)
	require.NoError(t, insertMCPProjectData(projectBDB, urlB, hostB, riskTitleB))
	require.NoError(t, projectBDB.Close())

	queryReq := rawmcp.CallToolRequest{}
	queryReq.Params.Name = "query_http_flow"
	queryReq.Params.Arguments = map[string]any{}
	queryResult, err := mcpClient.CallTool(clientCtx, queryReq)
	require.NoError(t, err)
	requireMCPHTTPFlowURL(t, queryResult, urlA)
	requireMCPPortHost(t, mcpClient, clientCtx, hostA)
	requireMCPRiskTitle(t, mcpClient, clientCtx, riskTitleA)

	// Simulate the frontend switching projects after MCP has already started.
	// The existing MCP connection must resolve the newly active database on its
	// next tool call without restarting the MCP server.
	_, err = client.SetCurrentProject(clientCtx, &ypb.SetCurrentProjectRequest{
		Id:   idB,
		Type: yakit.TypeProject,
	})
	require.NoError(t, err)

	queryResult, err = mcpClient.CallTool(clientCtx, queryReq)
	require.NoError(t, err)
	requireMCPHTTPFlowURL(t, queryResult, urlB)
	requireMCPPortHost(t, mcpClient, clientCtx, hostB)
	requireMCPRiskTitle(t, mcpClient, clientCtx, riskTitleB)

	contextReq := rawmcp.CallToolRequest{}
	contextReq.Params.Name = "get_current_database_context"
	contextReq.Params.Arguments = map[string]any{}
	contextResult, err := mcpClient.CallTool(clientCtx, contextReq)
	require.NoError(t, err)
	contextData := mustExtractToolTextJSON(t, contextResult)
	currentProject = contextData["current_project"].(map[string]any)
	require.Equal(t, float64(idB), currentProject["Id"])
	require.NotEmpty(t, contextData["current_project_db_path"])
}

func TestGRPC_StartMcpServer_FollowsProjectSwitchFromDefault(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	testCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	originalProject, err := client.GetCurrentProject(testCtx, &ypb.Empty{})
	require.NoError(t, err)
	defaultProject, err := client.GetDefaultProject(testCtx, &ypb.Empty{})
	require.NoError(t, err)
	require.Greater(t, defaultProject.GetId(), int64(0))

	var (
		defaultFlowID   uint
		testProjectID   int64
		testProjectPath string
	)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()

		// Delete the default-database fixture explicitly because the default
		// project itself is persistent and must never be removed by a test.
		if defaultProject.GetId() > 0 {
			_, switchErr := client.SetCurrentProject(cleanupCtx, &ypb.SetCurrentProjectRequest{
				Id:   defaultProject.GetId(),
				Type: yakit.TypeProject,
			})
			if switchErr != nil {
				t.Errorf("cleanup: switch to default project failed: %v", switchErr)
			}
		}
		if defaultFlowID > 0 {
			defaultDB := consts.GetGormProjectDatabase()
			if deleteErr := defaultDB.Unscoped().Delete(&schema.HTTPFlow{}, defaultFlowID).Error; deleteErr != nil {
				t.Errorf("cleanup: delete default HTTP flow fixture failed: %v", deleteErr)
			}
			var remaining int
			if countErr := defaultDB.Unscoped().Model(&schema.HTTPFlow{}).Where("id = ?", defaultFlowID).Count(&remaining).Error; countErr != nil {
				t.Errorf("cleanup: verify default HTTP flow fixture failed: %v", countErr)
			} else if remaining != 0 {
				t.Errorf("cleanup: default HTTP flow fixture still exists: id=%d", defaultFlowID)
			}
		}
		if testProjectID > 0 {
			_, deleteErr := client.DeleteProject(cleanupCtx, &ypb.DeleteProjectRequest{
				Id:            testProjectID,
				IsDeleteLocal: true,
				Type:          yakit.TypeProject,
			})
			if deleteErr != nil {
				t.Errorf("cleanup: delete temporary project failed: %v", deleteErr)
			}
			if _, lookupErr := yakit.GetProjectByID(consts.GetGormProfileDatabase(), testProjectID); lookupErr == nil {
				t.Errorf("cleanup: temporary project record still exists: id=%d", testProjectID)
			}
			if testProjectPath != "" {
				if _, statErr := os.Stat(testProjectPath); !os.IsNotExist(statErr) {
					t.Errorf("cleanup: temporary project database still exists: path=%s err=%v", testProjectPath, statErr)
				}
			}
		}
		if originalProject != nil && originalProject.GetId() > 0 {
			_, restoreErr := client.SetCurrentProject(cleanupCtx, &ypb.SetCurrentProjectRequest{
				Id:   originalProject.GetId(),
				Type: yakit.TypeProject,
			})
			if restoreErr != nil {
				t.Errorf("cleanup: restore original project failed: %v", restoreErr)
			}
		}
	})

	_, err = client.SetCurrentProject(testCtx, &ypb.SetCurrentProjectRequest{
		Id:   defaultProject.GetId(),
		Type: yakit.TypeProject,
	})
	require.NoError(t, err)

	marker := "mcp-follow-project-" + utils.RandStringBytes(10)
	defaultURL := "https://default.example/" + marker
	projectURL := "https://project.example/" + marker
	defaultFlow := newMCPProjectHTTPFlow(defaultURL)
	require.NoError(t, yakit.InsertHTTPFlow(consts.GetGormProjectDatabase(), defaultFlow))
	defaultFlowID = defaultFlow.ID
	require.Greater(t, defaultFlowID, uint(0))

	testProject, err := client.NewProject(testCtx, &ypb.NewProjectRequest{
		ProjectName: "mcp_follow_project_" + utils.RandStringBytes(8),
		Description: "temporary MCP project-switch regression fixture",
		Type:        yakit.TypeProject,
	})
	require.NoError(t, err)
	require.Greater(t, testProject.GetId(), int64(0))
	testProjectID = testProject.GetId()
	testProjectRecord, err := yakit.GetProjectByID(consts.GetGormProfileDatabase(), testProject.GetId())
	require.NoError(t, err)
	require.NotEmpty(t, testProjectRecord.DatabasePath)
	testProjectPath = testProjectRecord.DatabasePath

	testProjectDB, err := consts.CreateProjectDatabase(testProjectRecord.DatabasePath)
	require.NoError(t, err)
	require.NoError(t, yakit.InsertHTTPFlow(testProjectDB, newMCPProjectHTTPFlow(projectURL)))
	require.NoError(t, testProjectDB.Close())

	stream, err := client.StartMcpServer(testCtx, &ypb.StartMcpServerRequest{
		Host: "127.0.0.1",
		Port: 0,
		Tool: []string{"project_database", "httpflow"},
	})
	require.NoError(t, err)

	var serverURL string
	for i := 0; i < 5; i++ {
		resp, recvErr := stream.Recv()
		require.NoError(t, recvErr)
		if resp.GetServerUrl() != "" {
			serverURL = resp.GetServerUrl()
		}
		if resp.GetStatus() == "running" {
			break
		}
	}
	require.NotEmpty(t, serverURL)

	mcpClient, err := mcpclient.NewSSEMCPClient(serverURL)
	require.NoError(t, err)
	defer mcpClient.Close()
	require.NoError(t, mcpClient.Start(testCtx))

	initRequest := rawmcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = rawmcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = rawmcp.Implementation{
		Name:    "test-mcp-project-follow-client",
		Version: "1.0.0",
	}
	_, err = mcpClient.Initialize(testCtx, initRequest)
	require.NoError(t, err)

	// MCP starts while default is active: only the default fixture is visible.
	requireMCPHTTPFlowSelection(t, mcpClient, testCtx, marker, defaultURL, projectURL)

	// Simulate Yakit's frontend switching to the newly created project.
	_, err = client.SetCurrentProject(testCtx, &ypb.SetCurrentProjectRequest{
		Id:   testProject.GetId(),
		Type: yakit.TypeProject,
	})
	require.NoError(t, err)
	requireMCPHTTPFlowSelection(t, mcpClient, testCtx, marker, projectURL, defaultURL)

	// The pre-existing MCP switch tool must continue to use the same shared
	// project state and can switch the running MCP connection back to default.
	switchResult := callMCPSwitchProject(t, mcpClient, testCtx, defaultProject.GetId())
	switchData := mustExtractToolTextJSON(t, switchResult)
	currentProject := switchData["current_project"].(map[string]any)
	require.Equal(t, float64(defaultProject.GetId()), currentProject["Id"])
	requireMCPHTTPFlowSelection(t, mcpClient, testCtx, marker, defaultURL, projectURL)

	// And switching through the MCP tool in the other direction remains valid.
	callMCPSwitchProject(t, mcpClient, testCtx, testProject.GetId())
	requireMCPHTTPFlowSelection(t, mcpClient, testCtx, marker, projectURL, defaultURL)
}

func newMCPProjectHTTPFlow(url string) *schema.HTTPFlow {
	return &schema.HTTPFlow{
		Url:        url,
		Path:       "/",
		Method:     "GET",
		StatusCode: 200,
		SourceType: schema.HTTPFlow_SourceType_MITM,
		Request:    "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
		Response:   "HTTP/1.1 200 OK\r\n\r\n",
	}
}

func callMCPSwitchProject(t *testing.T, client mcpclient.MCPClient, ctx context.Context, projectID int64) *rawmcp.CallToolResult {
	t.Helper()
	req := rawmcp.CallToolRequest{}
	req.Params.Name = "switch_current_project_database"
	req.Params.Arguments = map[string]any{"id": projectID}
	result, err := client.CallTool(ctx, req)
	require.NoError(t, err)
	return result
}

func requireMCPHTTPFlowSelection(
	t *testing.T,
	client mcpclient.MCPClient,
	ctx context.Context,
	marker, expectedURL, excludedURL string,
) {
	t.Helper()
	req := rawmcp.CallToolRequest{}
	req.Params.Name = "query_http_flow"
	req.Params.Arguments = map[string]any{"keyword": marker}
	result, err := client.CallTool(ctx, req)
	require.NoError(t, err)

	payload := mustExtractToolTextJSON(t, result)
	flows, ok := payload["flows"].([]any)
	require.True(t, ok)
	require.Len(t, flows, 1)
	flow, ok := flows[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, expectedURL, flow["url"])
	require.NotEqual(t, excludedURL, flow["url"])
}

func insertMCPProjectData(db *gorm.DB, url, host, riskTitle string) error {
	if err := yakit.InsertHTTPFlow(db, newMCPProjectHTTPFlow(url)); err != nil {
		return err
	}
	if err := db.Create(&schema.Port{Host: host, Port: 443, Proto: "tcp", State: "open"}).Error; err != nil {
		return err
	}
	return db.Create(&schema.Risk{
		Hash:     utils.CalcSha1(riskTitle),
		Host:     host,
		Title:    riskTitle,
		Severity: "high",
	}).Error
}

func requireMCPHTTPFlowURL(t *testing.T, result *rawmcp.CallToolResult, expected string) {
	t.Helper()
	payload := mustExtractToolTextJSON(t, result)
	flows, ok := payload["flows"].([]any)
	require.True(t, ok)
	require.Len(t, flows, 1)
	flow, ok := flows[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, expected, flow["url"])
}

func requireMCPPortHost(t *testing.T, client mcpclient.MCPClient, ctx context.Context, expected string) {
	t.Helper()
	req := rawmcp.CallToolRequest{}
	req.Params.Name = "query_ports"
	req.Params.Arguments = map[string]any{
		"all":        true,
		"pagination": map[string]any{"page": 1, "limit": 10},
	}
	result, err := client.CallTool(ctx, req)
	require.NoError(t, err)
	ports := mustExtractToolJSONArray(t, result)
	require.Len(t, ports, 1)
	require.Equal(t, expected, ports[0]["Host"])
}

func requireMCPRiskTitle(t *testing.T, client mcpclient.MCPClient, ctx context.Context, expected string) {
	t.Helper()
	req := rawmcp.CallToolRequest{}
	req.Params.Name = "query_risks"
	req.Params.Arguments = map[string]any{
		"pagination": map[string]any{"page": 1, "limit": 10},
	}
	result, err := client.CallTool(ctx, req)
	require.NoError(t, err)
	payload := mustExtractToolTextJSON(t, result)
	risks, ok := payload["Data"].([]any)
	require.True(t, ok)
	require.Len(t, risks, 1)
	risk, ok := risks[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, expected, risk["Title"])
}

func TestGRPC_StartMcpServer_WithAIToolFramework(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req := &ypb.StartMcpServerRequest{
		Host:                  "127.0.0.1",
		Port:                  0,
		EnableAIToolFramework: true,
	}

	stream, err := client.StartMcpServer(ctx, req)
	require.NoError(t, err)

	var serverURL string
	for i := 0; i < 5; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if resp.GetServerUrl() != "" {
			serverURL = resp.GetServerUrl()
		}
		log.Infof("MCP Status: %s", resp.GetStatus())
		if resp.GetStatus() == "running" {
			break
		}
	}
	require.NotEmpty(t, serverURL, "server URL must be set")

	mcpClient, err := mcpclient.NewSSEMCPClient(serverURL)
	require.NoError(t, err)
	defer mcpClient.Close()

	clientCtx, clientCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer clientCancel()

	require.NoError(t, mcpClient.Start(clientCtx))

	initReq := rawmcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = rawmcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = rawmcp.Implementation{Name: "test-aitool-fw-client", Version: "1.0.0"}
	_, err = mcpClient.Initialize(clientCtx, initReq)
	require.NoError(t, err)

	toolsResult, err := mcpClient.ListTools(clientCtx, rawmcp.ListToolsRequest{})
	require.NoError(t, err)
	require.NotNil(t, toolsResult)

	// buildinaitools.GetAllToolsDynamically must register at least one tool.
	assert.Greater(t, len(toolsResult.Tools), 0,
		"EnableAIToolFramework must expose at least one built-in aitool-framework tool")
	log.Infof("EnableAIToolFramework: got %d tools", len(toolsResult.Tools))
}

func mustExtractToolTextJSON(t *testing.T, result *rawmcp.CallToolResult) map[string]any {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)

	first, ok := result.Content[0].(map[string]any)
	require.True(t, ok, "tool content item should be map[string]any")
	text, ok := first["text"].(string)
	require.True(t, ok, "tool content text should be string")

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	return out
}

func mustExtractToolJSONArray(t *testing.T, result *rawmcp.CallToolResult) []map[string]any {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)

	first, ok := result.Content[0].(map[string]any)
	require.True(t, ok, "tool content item should be map[string]any")
	text, ok := first["text"].(string)
	require.True(t, ok, "tool content text should be string")

	var out []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	return out
}

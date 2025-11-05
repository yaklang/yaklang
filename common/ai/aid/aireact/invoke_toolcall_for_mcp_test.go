package aireact

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// mockedMCPToolCalling mocks the AI responses for MCP tool calling
// nonce is used to verify that the AI reads the schema correctly
func mockedMCPToolCalling(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string, nonce string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for mcp tool calling", "cumulative_summary": "..cumulative-mocked for mcp tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		// Verify that the prompt contains the nonce in the schema
		if !strings.Contains(prompt, nonce) {
			return nil, utils.Errorf("SECURITY CHECK FAILED: prompt does not contain nonce %s, schema was not properly included", nonce)
		}

		rsp := i.NewAIResponse()
		// Generate message with nonce to prove we read the schema
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "message" : "message_` + nonce + `" }}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason for mcp"}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_MCPToolUse(t *testing.T) {
	// Generate a unique nonce for this test run - this is used to verify schema is properly included
	nonce := ksuid.New().String()
	log.Infof("Test nonce generated: %s", nonce)

	// Test server name
	serverName := "test_react_mcp_server_" + ksuid.New().String()

	// Clean up function: remove database record after test
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Fatal("profile database is nil")
	}

	// Clean up any existing test server
	var oldServer schema.MCPServer
	if err := db.Where("name = ?", serverName).First(&oldServer).Error; err == nil {
		db.Unscoped().Delete(&oldServer)
		log.Infof("cleaned up old test mcp server: %s", serverName)
	}

	defer func() {
		var server schema.MCPServer
		if err := db.Where("name = ?", serverName).First(&server).Error; err == nil {
			db.Unscoped().Delete(&server)
			log.Infof("cleaned up test mcp server: %s", serverName)
		}
	}()

	// Step 1: Create and start a real SSE MCP server
	mcpServer := server.NewMCPServer(
		"Test ReAct MCP Server",
		"1.0.0",
	)

	// Track if the MCP tool was called and the message it received
	mcpToolCalled := false
	var mcpToolMessage string
	var mcpToolCalledMutex sync.Mutex

	// Add a test tool to MCP server with nonce in description
	// This verifies that the schema is properly passed to the AI
	testTool := mcp.NewTool(
		"test_echo",
		mcp.WithDescription(fmt.Sprintf("A simple echo tool for ReAct testing. Use nonce: %s", nonce)),
		mcp.WithString("message", mcp.Description(fmt.Sprintf("The message to echo, should be in format: message_%s", nonce)), mcp.Required()),
	)

	// Register tool handler
	mcpServer.AddTool(testTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		mcpToolCalledMutex.Lock()
		mcpToolCalled = true
		message, ok := request.Params.Arguments["message"].(string)
		if ok {
			mcpToolMessage = message
		}
		mcpToolCalledMutex.Unlock()

		if !ok {
			return &mcp.CallToolResult{
				Content: []any{mcp.TextContent{Type: "text", Text: "Error: message must be a string"}},
				IsError: true,
			}, nil
		}
		log.Infof("MCP tool test_echo called with message: %s", message)
		return &mcp.CallToolResult{
			Content: []any{mcp.TextContent{Type: "text", Text: "MCP Echo: " + message}},
			IsError: false,
		}, nil
	})

	// Get random available port
	port := utils.GetRandomAvailableTCPPort()
	host := "127.0.0.1"
	hostPort := utils.HostPort(host, port)
	baseURL := fmt.Sprintf("http://%s", hostPort)
	sseURL := baseURL + "/sse"

	log.Infof("Starting SSE MCP server on %s", sseURL)

	// Create SSE server
	sseServer := server.NewSSEServer(mcpServer, baseURL)

	// Start server in background
	serverStarted := make(chan struct{})
	go func() {
		close(serverStarted)
		if err := sseServer.Start(hostPort); err != nil && err != http.ErrServerClosed {
			log.Errorf("SSE server error: %v", err)
		}
	}()

	// Wait for server to start
	<-serverStarted
	time.Sleep(100 * time.Millisecond)
	err := utils.WaitConnect(hostPort, 5)
	if err != nil {
		t.Fatalf("failed to wait for server to start: %v", err)
	}

	log.Infof("SSE MCP server started successfully on %s", sseURL)

	// Step 2: Save MCP Server configuration to database with Enable=true
	mcpServerConfig := &schema.MCPServer{
		Name:   serverName,
		Type:   "sse",
		URL:    sseURL,
		Enable: true, // Important: must be enabled
	}

	err = yakit.CreateMCPServer(db, mcpServerConfig)
	if err != nil {
		t.Fatalf("failed to create mcp server in database: %v", err)
	}

	// Verify it was saved correctly
	var savedServer schema.MCPServer
	err = db.Where("name = ?", serverName).First(&savedServer).Error
	if err != nil {
		t.Fatalf("failed to query saved mcp server: %v", err)
	}
	if !savedServer.Enable {
		t.Fatal("MCP server should be enabled")
	}

	log.Infof("MCP Server config saved to database: %s (Enable=%v)", serverName, savedServer.Enable)

	// Step 3: Create ReAct instance with MCP servers enabled
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// Track MCP loading stream events
	mcpLoadingStarted := false
	mcpLoadingDone := false
	var mcpLoadingMutex sync.Mutex

	// The full MCP tool name will be: mcp_{serverName}_test_echo
	mcpToolName := fmt.Sprintf("mcp_%s_test_echo", serverName)

	ins, err := NewReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedMCPToolCalling(i, r, mcpToolName, nonce)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			// Check for MCP loading stream events
			if e.NodeId == "mcp-loader" {
				mcpLoadingMutex.Lock()
				if e.IsStream {
					content := string(e.StreamDelta)
					log.Infof("MCP Loader Stream: %s", content)
					if strings.Contains(content, "Loading AI tools from MCP server") {
						mcpLoadingStarted = true
					}
					if strings.Contains(content, "Loaded AI tools from MCP servers") {
						mcpLoadingDone = true
					}
				}
				mcpLoadingMutex.Unlock()
			}
			out <- e.ToGRPC()
		}),
		aicommon.WithMemoryTriage(nil),
		aicommon.WithEnableSelfReflection(false),
		aicommon.WithDisallowMCPServers(false), // Important: enable MCP servers
	)
	if err != nil {
		t.Fatalf("failed to create ReAct instance: %v", err)
	}

	// Wait a bit for MCP tools to be loaded asynchronously
	log.Infof("Waiting for MCP tools to be loaded...")
	time.Sleep(3 * time.Second)

	// Verify that MCP loading streams were emitted
	mcpLoadingMutex.Lock()
	if !mcpLoadingStarted {
		mcpLoadingMutex.Unlock()
		t.Fatal("Expected MCP loading start stream event, but got none")
	}
	if !mcpLoadingDone {
		mcpLoadingMutex.Unlock()
		t.Fatal("Expected MCP loading done stream event, but got none")
	}
	mcpLoadingMutex.Unlock()

	log.Infof("MCP tools loaded successfully, checking if tool is available...")

	// Verify the MCP tool was loaded
	toolManager := ins.config.GetAiToolManager()
	allTools, err := toolManager.GetEnableTools()
	if err != nil {
		t.Fatalf("failed to get enabled tools: %v", err)
	}
	mcpToolFound := false
	for _, tool := range allTools {
		if tool.Name == mcpToolName {
			mcpToolFound = true
			log.Infof("Found MCP tool: %s", tool.Name)
			break
		}
	}
	if !mcpToolFound {
		t.Fatalf("MCP tool %s not found in tool manager. Available tools: %d", mcpToolName, len(allTools))
	}

	// Step 4: Send input to trigger tool calling
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test mcp tool",
		}
	}()

	du := time.Duration(15)
	if utils.InGithubActions() {
		du = time.Duration(10)
	}
	after := time.After(du * time.Second)

	reviewed := false
	reviewReleased := false
	toolCallOutputEvent := false
	var iid string

LOOP:
	for {
		select {
		case e := <-out:
			if e.IsStream {
				if e.ContentType == "" {
					t.Fatal("stream event should have content type")
				}
				if utils.IsNil(e.GetNodeIdVerbose()) {
					t.Fatal("node id should not be nil")
				}
				fmt.Println(string(e.GetStreamDelta()))
			}
			fmt.Println(e.String())

			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				reviewed = true
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        utils.InterfaceToString(iid),
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			if e.Type == string(schema.EVENT_TYPE_REVIEW_RELEASE) {
				gotId := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				if gotId == iid {
					reviewReleased = true
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallOutputEvent = true
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	// Step 5: Verify results
	if !reviewed {
		t.Fatal("Expected to have at least one review event, but got none")
	}

	if !reviewReleased {
		t.Fatal("Expected to have at least one review release event, but got none")
	}

	mcpToolCalledMutex.Lock()
	if !mcpToolCalled {
		mcpToolCalledMutex.Unlock()
		t.Fatal("MCP tool was not called")
	}
	// SECURITY CHECK: Verify the MCP tool received the correct message with nonce
	expectedMessage := fmt.Sprintf("message_%s", nonce)
	if mcpToolMessage != expectedMessage {
		mcpToolCalledMutex.Unlock()
		t.Fatalf("SECURITY CHECK FAILED: MCP tool received message '%s', expected '%s'. This means the schema was not properly read!",
			mcpToolMessage, expectedMessage)
	}
	log.Infof("✓ SECURITY CHECK PASSED: MCP tool received correct message with nonce: %s", mcpToolMessage)
	mcpToolCalledMutex.Unlock()

	if !toolCallOutputEvent {
		t.Fatal("Expected to have at least one tool call output event, but got none")
	}

	// Verify timeline contains MCP tool call information
	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	if !strings.Contains(tl, `mocked thought for mcp tool calling`) {
		t.Fatal("timeline does not contain mocked thought for mcp tool calling")
	}
	if !utils.MatchAllOfSubString(tl, `system-question`, "user-answer", "when review") {
		t.Fatal("timeline does not contain system-question")
	}
	if !utils.MatchAllOfSubString(tl, `ReAct iteration 1`, `ReAct Iteration Done[1]`) {
		t.Fatal("timeline does not contain ReAct iteration")
	}

	// Verify timeline contains the nonce-based message
	if !strings.Contains(tl, expectedMessage) {
		t.Fatalf("timeline does not contain the expected message with nonce: %s", expectedMessage)
	}
	log.Infof("✓ Timeline verification passed: contains message with nonce")

	fmt.Println("--------------------------------------")

	log.Infof("✓✓✓ Test completed successfully! All security checks passed! ✓✓✓")
}

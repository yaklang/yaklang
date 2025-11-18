package aireact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// setupMockToolInDB creates a mock tool and saves it to the database
func setupMockToolInDB(t *testing.T, toolName string) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Fatal("database not initialized")
	}

	// First try to delete existing tool with same name
	cleanupMockToolFromDB(t, toolName)

	// Create a mock YakScript tool content
	toolContent := `# tool-name: ` + toolName + `
# tool-description: A mock test tool for database testing
# tool-keywords: mock,test,database

yakit.Info("Mock tool %s executed successfully", "` + toolName + `")
message = cli.String("message", cli.setDefault("Hello from mock tool"))
yakit.Info("Message: %s", message)
`

	// Parse and create the tool
	aiTool := yakscripttools.LoadYakScriptToAiTools(toolName, toolContent)
	if aiTool == nil {
		t.Fatalf("failed to load yak script to ai tool")
	}

	// Save to database
	_, err := yakit.CreateAIYakTool(db, aiTool)
	if err != nil {
		t.Fatalf("failed to save ai tool to database: %v", err)
	}

	log.Infof("created mock tool in database: %s", toolName)
}

// cleanupMockToolFromDB removes the mock tool from database
func cleanupMockToolFromDB(t *testing.T, toolName string) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return
	}

	_, err := yakit.DeleteAIYakTools(db, toolName)
	if err != nil {
		log.Warnf("failed to delete mock tool from database: %v", err)
	}
}

// createMockToolSearcherForDB creates a searcher that searches from database tools
func createMockToolSearcherForDB() searchtools.AISearcher[*aitool.Tool] {
	return func(query string, searchList []*aitool.Tool) ([]*aitool.Tool, error) {
		var results []*aitool.Tool
		queryLower := strings.ToLower(query)

		log.Infof("searching tools with query: %s, total tools: %d", query, len(searchList))

		for _, item := range searchList {
			nameLower := strings.ToLower(item.GetName())
			descLower := strings.ToLower(item.GetDescription())

			// Also check keywords
			keywordsMatch := false
			for _, kw := range item.GetKeywords() {
				if strings.Contains(strings.ToLower(kw), queryLower) {
					keywordsMatch = true
					break
				}
			}

			if strings.Contains(nameLower, queryLower) ||
				strings.Contains(descLower, queryLower) ||
				keywordsMatch {
				results = append(results, item)
				log.Infof("found matching tool: %s", item.GetName())
			}
		}

		if len(results) == 0 {
			log.Warnf("no tools found for query: %s", query)
		}

		return results, nil
	}
}

// mockedToolCallingForDB mocks AI responses for database tool testing
func mockedToolCallingForDB(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	// First stage: decide to use tool_search or direct tool
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for tool calling from database", "cumulative_summary": "..cumulative-mocked for db tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	// Second stage: generate parameters for the tool
	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		rsp := i.NewAIResponse()
		// For tools_search, provide query parameter
		if strings.Contains(prompt, "tools_search") {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "query" : "mock_db_tool" }}`))
		} else {
			// For the actual mock tool, provide message parameter
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "message" : "test message" }}`))
		}
		rsp.Close()
		return rsp, nil
	}

	// Third stage: verify satisfaction
	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason-db-tool"}`))
		rsp.Close()
		return rsp, nil
	}

	log.Errorf("unexpected prompt in mockedToolCallingForDB: %s", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

// TestReAct_ToolUse_FromDB_ViaToolSearch tests calling a tool from database via tool_search
func TestReAct_ToolUse_FromDB_ViaToolSearch(t *testing.T) {
	toolName := fmt.Sprintf("mock_db_tool_search_%d", time.Now().Unix())

	// Setup: create mock tool in database
	setupMockToolInDB(t, toolName)
	defer cleanupMockToolFromDB(t, toolName)

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolSearchCalled := false
	mockToolCalled := false
	searchQuery := ""

	// Create tool manager with search enabled and explicitly enable the mock tool
	toolManager := buildinaitools.NewToolManager(
		buildinaitools.WithSearchToolEnabled(true),
		buildinaitools.WithAIToolsSearcher(createMockToolSearcherForDB()),
	)
	// Enable the mock tool so it can be called after search
	toolManager.EnableTool(toolName)

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// First: AI decides to use tools_search
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				// Check if we've already called tools_search
				if !toolSearchCalled {
					rsp := i.NewAIResponse()
					rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "tools_search" },
"human_readable_thought": "need to search for the mock tool", "cumulative_summary": "searching for tools"}
`))
					rsp.Close()
					return rsp, nil
				} else {
					// After tools_search, call the actual mock tool
					return mockedToolCallingForDB(i, r, toolName)
				}
			}

			// Generate parameters
			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				rsp := i.NewAIResponse()
				if strings.Contains(prompt, "tools_search") {
					// Provide search query
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "query" : "mock_db_tool" }}`))
				} else {
					// Provide parameters for the actual tool
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "message" : "test from search" }}`))
				}
				rsp.Close()
				return rsp, nil
			}

			// Verify satisfaction
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				// After tools_search, we should continue to call the actual tool
				if toolSearchCalled && !mockToolCalled {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "found tools, now need to call the actual tool"}`))
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "tool executed successfully"}`))
				}
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithToolManager(toolManager),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "use tool search to find and execute mock_db_tool",
		}
	}()

	du := time.Duration(15)
	if utils.InGithubActions() {
		du = time.Duration(10)
	}
	after := time.After(du * time.Second)

	var iid string
LOOP:
	for {
		select {
		case e := <-out:
			if e.IsStream {
				fmt.Print(string(e.GetStreamDelta()))
			}

			// Handle tool review
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				toolNameInEvent := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.tool"))

				log.Infof("tool review required for: %s", toolNameInEvent)

				if toolNameInEvent == "tools_search" {
					toolSearchCalled = true
					// Extract search query
					paramsJSON := jsonpath.FindFirst(string(e.Content), "$.params")
					if paramsJSON != nil {
						paramsBytes, _ := json.Marshal(paramsJSON)
						var params map[string]interface{}
						if err := json.Unmarshal(paramsBytes, &params); err == nil {
							if q, ok := params["query"].(string); ok {
								searchQuery = q
								log.Infof("captured search query: %s", searchQuery)
							}
						}
					}
				} else if strings.Contains(toolNameInEvent, "mock_db_tool") {
					mockToolCalled = true
					log.Infof("mock tool called: %s", toolNameInEvent)
				}

				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				status := utils.InterfaceToString(result)
				log.Infof("task status: %s", status)
				if status == "completed" || status == "failed" {
					break LOOP
				}
			}
		case <-after:
			log.Warnf("test timeout")
			break LOOP
		}
	}
	close(in)

	// Verify results
	if !toolSearchCalled {
		t.Fatal("tools_search was not called")
	}

	if searchQuery == "" {
		t.Fatal("search query was not captured")
	}

	if !mockToolCalled {
		t.Fatal("mock tool from database was not called after search")
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)

	if !strings.Contains(tl, "tools_search") {
		t.Fatal("timeline does not contain tools_search")
	}

	if !strings.Contains(tl, toolName) {
		t.Fatal("timeline does not contain mock tool name")
	}

	fmt.Println("--------------------------------------")
	fmt.Printf("✓ Successfully called tool from database via tool_search\n")
	fmt.Printf("  Search query: %s\n", searchQuery)
	fmt.Printf("  Tool called: %s\n", toolName)
}

// TestReAct_ToolUse_FromDB_DirectCall tests calling a tool from database directly by name
func TestReAct_ToolUse_FromDB_DirectCall(t *testing.T) {
	toolName := fmt.Sprintf("mock_db_tool_direct_%d", time.Now().Unix())

	// Setup: create mock tool in database
	setupMockToolInDB(t, toolName)
	defer cleanupMockToolFromDB(t, toolName)

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	mockToolCalled := false

	// Create tool manager and explicitly enable the mock tool from database
	toolManager := buildinaitools.NewToolManager()
	toolManager.EnableTool(toolName)

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingForDB(i, r, toolName)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithToolManager(toolManager),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "execute " + toolName,
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var iid string
	reviewed := false
	reviewReleased := false
	toolCallOutputEvent := false

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
				fmt.Print(string(e.GetStreamDelta()))
			}

			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				reviewed = true
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				toolNameInEvent := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.tool"))

				log.Infof("tool review required for: %s", toolNameInEvent)

				if strings.Contains(toolNameInEvent, toolName) {
					mockToolCalled = true
					log.Infof("mock tool called directly: %s", toolNameInEvent)
				}

				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
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
				status := utils.InterfaceToString(result)
				log.Infof("task status: %s", status)
				if status == "completed" || status == "failed" {
					break LOOP
				}
			}
		case <-after:
			log.Warnf("test timeout")
			break LOOP
		}
	}
	close(in)

	// Verify results
	if !reviewed {
		t.Fatal("Expected to have at least one review event, but got none")
	}

	if !reviewReleased {
		t.Fatal("Expected to have at least one review release event, but got none")
	}

	if !mockToolCalled {
		t.Fatal("Mock tool from database was not called")
	}

	if !toolCallOutputEvent {
		t.Fatal("Expected to have at least one tool call output event, but got none")
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)

	if !strings.Contains(tl, "mocked thought for tool calling from database") {
		t.Fatal("timeline does not contain mocked thought")
	}

	if !utils.MatchAllOfSubString(tl, "system-question", "user-answer", "when review") {
		t.Fatal("timeline does not contain system-question")
	}

	if !utils.MatchAllOfSubString(tl, "ReAct iteration 1", "ReAct Iteration Done[1]") {
		t.Fatal("timeline does not contain ReAct iteration")
	}

	fmt.Println("--------------------------------------")
	fmt.Printf("✓ Successfully called tool from database directly by name: %s\n", toolName)
}

// TestReAct_ToolUse_WithNextMovements tests that next_movements appears in timeline when verify returns false
func TestReAct_ToolUse_WithNextMovements(t *testing.T) {
	toolName := fmt.Sprintf("mock_tool_next_movements_%d", time.Now().Unix())

	// Setup: create mock tool in database
	setupMockToolInDB(t, toolName)
	defer cleanupMockToolFromDB(t, toolName)

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolCalled := false
	verifyTriggered := false
	nextMovementsReceived := false
	nextMovementsContent := "Next, I need to use another tool to complete the task"

	// Create tool manager and enable the tool
	toolManager := buildinaitools.NewToolManager()
	toolManager.EnableTool(toolName)

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// First: AI decides to use the tool
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "calling the mock tool", "cumulative_summary": "tool execution"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Second: Generate parameters for the tool
			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "message" : "test message" }}`))
				rsp.Close()
				return rsp, nil
			}

			// Third: Verify satisfaction - return false with next_movements
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				verifyTriggered = true
				rsp := i.NewAIResponse()
				// Return user_satisfied=false with next_movements
				rsp.EmitOutputStream(bytes.NewBufferString(`{
"@action": "verify-satisfaction", 
"user_satisfied": false, 
"reasoning": "tool executed but task not complete",
"human_readable_result": "tool was called successfully",
"next_movements": "` + nextMovementsContent + `"
}`))
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithToolManager(toolManager),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "execute " + toolName,
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var iid string
LOOP:
	for {
		select {
		case e := <-out:
			if e.IsStream {
				streamContent := string(e.GetStreamDelta())
				// Check if next_movements content appears in stream
				if strings.Contains(streamContent, nextMovementsContent) {
					nextMovementsReceived = true
					log.Infof("received next_movements in stream: %s", streamContent)
				}
			}

			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				toolNameInEvent := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.tool"))

				if strings.Contains(toolNameInEvent, toolName) {
					toolCalled = true
					log.Infof("tool called: %s", toolNameInEvent)
				}

				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				status := utils.InterfaceToString(result)
				log.Infof("task status: %s", status)
				// Since user_satisfied=false, task should continue or eventually timeout
				if status == "completed" || status == "failed" || status == "aborted" {
					break LOOP
				}
			}
		case <-after:
			log.Warnf("test timeout - this is expected as user_satisfied=false")
			break LOOP
		}
	}
	close(in)

	// Verify test requirements
	if !toolCalled {
		t.Fatal("Requirement 1 failed: Tool was not called")
	}
	log.Infof("✓ Requirement 1 passed: Tool was called")

	if !verifyTriggered {
		t.Fatal("Requirement 2 failed: Verify was not triggered after tool call")
	}
	log.Infof("✓ Requirement 2 passed: Verify was triggered")

	if !nextMovementsReceived {
		t.Fatal("Requirement 3 failed: next_movements content was not received in stream")
	}
	log.Infof("✓ Requirement 3 passed: next_movements content was received")

	// Check timeline for next_movements
	timeline := ins.DumpTimeline()
	if !strings.Contains(timeline, "NEXT_MOVEMENTS") {
		t.Fatal("Requirement 4 failed: next_movements marker not found in timeline")
	}
	if !strings.Contains(timeline, nextMovementsContent) {
		t.Fatalf("Requirement 4 failed: next_movements content '%s' not found in timeline", nextMovementsContent)
	}
	log.Infof("✓ Requirement 4 passed: next_movements appears in timeline")

	fmt.Println("--------------------------------------")
	fmt.Println("Timeline:")
	fmt.Println(timeline)
	fmt.Println("--------------------------------------")
	fmt.Printf("✓ All requirements passed for next_movements test\n")
	fmt.Printf("  Tool called: %s\n", toolName)
	fmt.Printf("  Verify triggered: %v\n", verifyTriggered)
	fmt.Printf("  Next movements content: %s\n", nextMovementsContent)
}

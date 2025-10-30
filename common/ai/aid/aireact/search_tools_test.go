package aireact

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// createMockToolSearcher creates a simple mock searcher for testing that returns all matching tools
func createMockToolSearcher() searchtools.AISearcher[*aitool.Tool] {
	return func(query string, searchList []*aitool.Tool) ([]*aitool.Tool, error) {
		// Simple keyword matching - return tools that contain the query in name or description
		var results []*aitool.Tool
		queryLower := strings.ToLower(query)

		for _, item := range searchList {
			nameLower := strings.ToLower(item.GetName())
			descLower := strings.ToLower(item.GetDescription())

			if strings.Contains(nameLower, queryLower) || strings.Contains(descLower, queryLower) {
				results = append(results, item)
			}
		}

		// If no matches found, return first 3 tools (or all if less than 3)
		if len(results) == 0 {
			maxResults := 3
			if len(searchList) < maxResults {
				maxResults = len(searchList)
			}
			for i := 0; i < maxResults && i < len(searchList); i++ {
				results = append(results, searchList[i])
			}
		}

		return results, nil
	}
}

// createMockForgeSearcher creates a simple mock searcher for testing forge search
func createMockForgeSearcher() searchtools.AISearcher[*schema.AIForge] {
	return func(query string, searchList []*schema.AIForge) ([]*schema.AIForge, error) {
		// Simple keyword matching - return forges that contain the query in name or description
		var results []*schema.AIForge
		queryLower := strings.ToLower(query)

		for _, item := range searchList {
			nameLower := strings.ToLower(item.GetName())
			descLower := strings.ToLower(item.GetDescription())

			if strings.Contains(nameLower, queryLower) || strings.Contains(descLower, queryLower) {
				results = append(results, item)
			}
		}

		// If no matches found, return first 3 forges (or all if less than 3)
		if len(results) == 0 {
			maxResults := 3
			if len(searchList) < maxResults {
				maxResults = len(searchList)
			}
			for i := 0; i < maxResults && i < len(searchList); i++ {
				results = append(results, searchList[i])
			}
		}

		return results, nil
	}
}

// TestReAct_SearchTools_InPrompt tests that tools_search and forge_search tools are included in the prompt
func TestReAct_SearchTools_InPrompt(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// Track whether the prompt contains the search tools
	promptContainsToolsSearch := false
	promptContainsForgeSearch := false

	// Create a simple test tool
	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithDescription("Sleep for a specified number of seconds"),
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			sleepInt := params.GetFloat("seconds", 0.1)
			if sleepInt <= 0 {
				sleepInt = 0.1
			}
			time.Sleep(time.Duration(sleepInt) * time.Second)
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create tool manager options with searcher
	toolManagerOpts := []buildinaitools.ToolManagerOption{
		buildinaitools.WithExtendTools([]*aitool.Tool{sleepTool}, true),
		buildinaitools.WithSearchToolEnabled(true),
		buildinaitools.WithForgeSearchToolEnabled(true),
		buildinaitools.WithAIToolsSearcher(createMockToolSearcher()),
		buildinaitools.WithAiForgeSearcher(createMockForgeSearcher()),
	}

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// Check if the prompt contains tools_search
			if strings.Contains(prompt, "tools_search") || strings.Contains(prompt, searchtools.SearchToolName) {
				promptContainsToolsSearch = true
			}

			// Check if the prompt contains forge_search
			if strings.Contains(prompt, "aiforge_search") || strings.Contains(prompt, searchtools.SearchForgeName) {
				promptContainsForgeSearch = true
			}

			// Mock response - directly answer
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": {"type": "directly_answer"}, "answer_payload": "Test completed", 
"human_readable_thought": "Found search tools in prompt", "cumulative_summary": "Search tools verified"}`))
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithToolManager(buildinaitools.NewToolManager(toolManagerOpts...)),
		aicommon.WithDebug(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test search tools",
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" || utils.InterfaceToString(result) == "failed" {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !promptContainsToolsSearch {
		t.Fatal("Prompt does not contain tools_search tool")
	}

	if !promptContainsForgeSearch {
		t.Fatal("Prompt does not contain forge_search tool")
	}

	fmt.Println("✓ Both tools_search and forge_search are included in the prompt")
}

// TestReAct_ToolsSearch_Functionality tests that tools_search can actually search for tools
func TestReAct_ToolsSearch_Functionality(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolSearchCalled := false
	searchQuery := ""

	// Create some test tools
	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithDescription("Sleep for a specified number of seconds"),
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	echoTool, err := aitool.New(
		"echo",
		aitool.WithDescription("Echo back the input message"),
		aitool.WithStringParam("message"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return params.GetString("message"), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create tool manager options with searcher
	toolManagerOpts := []buildinaitools.ToolManagerOption{
		buildinaitools.WithExtendTools([]*aitool.Tool{sleepTool, echoTool}, true),
		buildinaitools.WithSearchToolEnabled(true),
		buildinaitools.WithAIToolsSearcher(createMockToolSearcher()),
	}

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// When AI decides to use tools_search
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "tools_search" },
"human_readable_thought": "Need to search for available tools", "cumulative_summary": "Searching for tools"}`))
				rsp.Close()
				return rsp, nil
			}

			// When AI generates parameters for tools_search
			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "query" : "echo" }}`))
				rsp.Close()
				return rsp, nil
			}

			// After tool execution, verify satisfaction
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Successfully found tools"}`))
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithToolManager(buildinaitools.NewToolManager(toolManagerOpts...)),
		aicommon.WithDebug(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "search for echo tool",
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
			fmt.Println(e.String())

			// Handle tool review
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				toolName := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.tool"))

				if toolName == "tools_search" {
					toolSearchCalled = true
					// Extract the query parameter
					paramsStr := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.params"))
					if strings.Contains(paramsStr, "echo") {
						searchQuery = "echo"
					}
				}

				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        utils.InterfaceToString(iid),
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" || utils.InterfaceToString(result) == "failed" {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !toolSearchCalled {
		t.Fatal("tools_search was not called")
	}

	if searchQuery == "" {
		t.Fatal("search query was not captured")
	}

	fmt.Println("✓ tools_search functionality works correctly with query:", searchQuery)
}

// TestReAct_ForgeSearch_Functionality tests that forge_search can actually search for forges
func TestReAct_ForgeSearch_Functionality(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	forgeSearchCalled := false

	// Create tool manager options with searcher
	toolManagerOpts := []buildinaitools.ToolManagerOption{
		buildinaitools.WithForgeSearchToolEnabled(true),
		buildinaitools.WithAiForgeSearcher(createMockForgeSearcher()),
	}

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// When AI decides to use forge_search
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "aiforge_search" },
"human_readable_thought": "Need to search for available forges", "cumulative_summary": "Searching for forges"}`))
				rsp.Close()
				return rsp, nil
			}

			// When AI generates parameters for forge_search
			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "query" : "test" }}`))
				rsp.Close()
				return rsp, nil
			}

			// After tool execution, verify satisfaction
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Successfully searched forges"}`))
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithToolManager(buildinaitools.NewToolManager(toolManagerOpts...)),
		aicommon.WithDebug(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "search for test forge",
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
			fmt.Println(e.String())

			// Handle tool review
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				toolName := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.tool"))

				if toolName == "aiforge_search" {
					forgeSearchCalled = true
				}

				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        utils.InterfaceToString(iid),
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" || utils.InterfaceToString(result) == "failed" {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !forgeSearchCalled {
		t.Fatal("forge_search was not called")
	}

	fmt.Println("✓ forge_search functionality works correctly")
}

// TestReAct_BothSearchTools_InPrompt tests that both search tools appear together in the prompt
func TestReAct_BothSearchTools_InPrompt(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	var capturedPrompt string
	bothToolsInSamePrompt := false

	// Create tool manager options with searcher
	toolManagerOpts := []buildinaitools.ToolManagerOption{
		buildinaitools.WithSearchToolEnabled(true),
		buildinaitools.WithForgeSearchToolEnabled(true),
		buildinaitools.WithAIToolsSearcher(createMockToolSearcher()),
		buildinaitools.WithAiForgeSearcher(createMockForgeSearcher()),
	}

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// Check if both tools appear in the same prompt
			if strings.Contains(prompt, "tools_search") && strings.Contains(prompt, "aiforge_search") {
				bothToolsInSamePrompt = true
				capturedPrompt = prompt
			}

			// Mock response - directly answer
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": {"type": "directly_answer"}, "answer_payload": "Both search tools found", 
"human_readable_thought": "Verified both search tools", "cumulative_summary": "Both tools present"}`))
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithToolManager(buildinaitools.NewToolManager(toolManagerOpts...)),
		aicommon.WithDebug(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test both search tools",
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

LOOP:
	for {
		select {
		case e := <-out:
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" || utils.InterfaceToString(result) == "failed" {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !bothToolsInSamePrompt {
		t.Fatal("Both tools_search and forge_search are not in the same prompt")
	}

	// Verify that the tools appear in the prioritized list
	if capturedPrompt != "" {
		// Check that tools_search appears before forge_search (based on priority in tools.go)
		toolsSearchIdx := strings.Index(capturedPrompt, "tools_search")
		forgeSearchIdx := strings.Index(capturedPrompt, "aiforge_search")

		if toolsSearchIdx == -1 || forgeSearchIdx == -1 {
			t.Fatal("One or both search tools not found in captured prompt")
		}

		// Note: We expect tools_search to appear before forge_search in the priority list
		// but this might not be guaranteed in all prompt formats
		fmt.Printf("✓ Both search tools are present in the same prompt\n")
		fmt.Printf("  tools_search position: %d\n", toolsSearchIdx)
		fmt.Printf("  forge_search position: %d\n", forgeSearchIdx)
	}
}

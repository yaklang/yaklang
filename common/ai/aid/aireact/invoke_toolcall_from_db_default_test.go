package aireact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"
)

// TestReAct_ToolUse_FromDB_ViaToolSearch_WithDefaultConfig tests invoking a tool stored in the database
func TestReAct_ToolUse_FromDB_ViaToolSearch_WithDefaultConfig(t *testing.T) {
	toolName := fmt.Sprintf("mock_db_tool_search_%s", utils.RandStringBytes(16))

	// Setup: create mock tool in database
	setupMockToolInDB(t, toolName)
	defer cleanupMockToolFromDB(t, toolName)

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolSearchCalled := false
	mockToolCalled := false
	searchQuery := ""

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

	du := time.Duration(150)
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

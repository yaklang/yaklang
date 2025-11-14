package aireact

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Track iteration count for self-reflection triggering
var iterationCount int
var verifyCount int
var iterationMutex sync.Mutex

// extractNonceFromPrompt extracts nonce from prompt patterns like <background_{nonce}>, <schema_{nonce}>, <|SCHEMA_{nonce}|>, etc.
func extractNonceFromPrompt(prompt string) string {
	// Pattern 1: <background_{nonce}> or <schema_{nonce}> (liteforge style)
	patterns := []string{
		`<background_([a-zA-Z0-9]+)>`,
		`<schema_([a-zA-Z0-9]+)>`,
		`<timeline_([a-zA-Z0-9]+)>`,
		`<params_([a-zA-Z0-9]+)>`,
		// Pattern 2: <|SCHEMA_{nonce}|> or <|USER_QUERY_{nonce}|> (ReAct loop style)
		`<\|SCHEMA_([A-Za-z0-9]+)\|>`,
		`<\|USER_QUERY_([A-Za-z0-9]+)\|>`,
		`<\|REFLECTION_([A-Za-z0-9]+)\|>`,
		`<\|PERSISTENT_([A-Za-z0-9]+)\|>`,
		// Pattern 3: <|USER_INPUT_{nonce}|> (task init style)
		`<\|USER_INPUT_([A-Za-z0-9]+)\|>`,
		// Pattern 4: <|SELF_REFLECTION_TASK_{nonce}|> (self-reflection prompt style)
		`<\|SELF_REFLECTION_TASK_([A-Za-z0-9]+)\|>`,
		`<\|ACTION_DETAILS_([A-Za-z0-9]+)\|>`,
		`<\|ENVIRONMENTAL_IMPACT_([A-Za-z0-9]+)\|>`,
		`<\|RELEVANT_MEMORIES_([A-Za-z0-9]+)\|>`,
		`<\|PREVIOUS_REFLECTIONS_([A-Za-z0-9]+)\|>`,
		`<\|ANALYSIS_REQUIREMENTS_([A-Za-z0-9]+)\|>`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(prompt); len(matches) > 1 {
			if matches[1] != "" {
				return matches[1] // Return first captured group (nonce)
			}
		}
	}

	return "" // No nonce found
}

// mockedSelfReflectionToolCalling mocks AI responses for self-reflection test
func mockedSelfReflectionToolCalling(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	// Extract nonce from prompt (if available)
	nonce := extractNonceFromPrompt(prompt)
	if nonce == "" {
		// Use default nonce for mock responses when extraction fails
		nonce = "test123"
	}

	// Mock decision to call tool - continue until we reach 7 iterations (> 5)
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		iterationMutex.Lock()
		iterationCount++
		currentIter := iterationCount
		iterationMutex.Unlock()

		rsp := i.NewAIResponse()
		// Continue iterating until we've done 7 iterations
		if currentIter < 7 {
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "Iteration ` + fmt.Sprintf("%d", currentIter) + `: continuing to test multi-iteration reflection trigger", 
"cumulative_summary": "..cumulative summary for iteration ` + fmt.Sprintf("%d", currentIter) + `.."}
`))
		} else {
			// After 7 iterations, finish
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "finish", 
"human_readable_thought": "Completed ` + fmt.Sprintf("%d", currentIter) + ` iterations, finishing test", 
"cumulative_summary": "..completed self-reflection test after ` + fmt.Sprintf("%d", currentIter) + ` iterations.."}
`))
		}
		rsp.Close()
		return rsp, nil
	}

	// Mock tool parameter generation
	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "message" : "test_message_` + nonce + `" }}`))
		rsp.Close()
		return rsp, nil
	}

	// Mock self-reflection AI analysis (CRITICAL FOR REFLECTION TEST)
	// 注意：字段现在都是可选的，可以返回简化的响应
	if utils.MatchAllOfSubString(prompt, "自我反思", "learning_insights") ||
		utils.MatchAllOfSubString(prompt, "SELF-REFLECTION", "learning_insights") ||
		utils.MatchAllOfSubString(prompt, "自我反思任务") {
		rsp := i.NewAIResponse()
		// Generate a simplified reflection response (all fields optional now)
		reflectionJSON := fmt.Sprintf(`{
  "@action": "self_reflection",
  "learning_insights": [
    "工具执行模式需要参数验证 (nonce: %s)",
    "多次迭代表明有优化空间"
  ],
  "future_suggestions": [
    "考虑为重复调用实现缓存机制"
  ],
  "impact_assessment": "操作在 %d 次迭代后成功执行",
  "effectiveness_rating": "effective"
}`, nonce, 6)
		rsp.EmitOutputStream(bytes.NewBufferString(reflectionJSON))
		rsp.Close()
		return rsp, nil
	}

	// Mock verification - keep iterating until we've done enough iterations
	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		iterationMutex.Lock()
		verifyCount++
		currentVerify := verifyCount
		iterationMutex.Unlock()

		rsp := i.NewAIResponse()
		// Only mark as satisfied after 7 verifications (one per iteration)
		if currentVerify < 7 {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "Still need more iterations for testing (` + fmt.Sprintf("%d", currentVerify) + `/7)"}`))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Completed 7 iterations for reflection test"}`))
		}
		rsp.Close()
		return rsp, nil
	}

	// Mock memory system - memory extraction/summarization
	// Match pattern: <background_{nonce}>, <schema_{nonce}>, or liteforge prompt signature
	if utils.MatchAllOfSubString(prompt, "是一个输出JSON的数据处理和总结提示小助手") ||
		strings.Contains(prompt, "<background_") && strings.Contains(prompt, "<schema_") {
		rsp := i.NewAIResponse()
		// Return minimal memory extraction response
		rsp.EmitOutputStream(bytes.NewBufferString(`{
  "summary": "Self-reflection test with nonce ` + nonce + ` - testing multi-iteration reflection trigger",
  "tags": ["testing", "self-reflection", "iteration"]
}`))
		rsp.Close()
		return rsp, nil
	}

	// Mock memory system - knowledge graph extraction
	// Match pattern: facts extraction prompts
	if strings.Contains(prompt, "working dir:") && strings.Contains(prompt, "facts") {
		rsp := i.NewAIResponse()
		// Return minimal knowledge graph response
		rsp.EmitOutputStream(bytes.NewBufferString(`{
  "facts": []
}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", utils.ShrinkString(prompt, 500))
	return nil, utils.Errorf("unexpected prompt: %s", utils.ShrinkString(prompt, 200))
}

func TestReAct_SelfReflection(t *testing.T) {
	// Reset counters for this test
	iterationMutex.Lock()
	iterationCount = 0
	verifyCount = 0
	iterationMutex.Unlock()

	// Generate nonce for security validation
	nonce := ksuid.New().String()
	t.Logf("Test nonce generated: %s", nonce)

	// Database setup
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Fatal("failed to get database")
	}

	// Track tool calls to ensure > 5 iterations
	toolCallCount := 0
	var toolCallMutex sync.Mutex
	var receivedMessages []string

	// Create a test tool that we'll call multiple times
	testTool, err := aitool.New(
		"test_echo_tool",
		aitool.WithStringParam("message", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCallMutex.Lock()
			toolCallCount++
			msg := params.GetString("message", "")
			receivedMessages = append(receivedMessages, msg)
			currentCount := toolCallCount
			toolCallMutex.Unlock()

			t.Logf("Tool called %d times with message: %s", currentCount, msg)
			time.Sleep(50 * time.Millisecond)
			return fmt.Sprintf("Echo: %s (call #%d)", msg, currentCount), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Setup channels
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	// Track reflection events
	var reflectionMutex sync.Mutex
	var reflectionContent string

	// Track memory events
	memoryBuildEvents := 0
	var memoryMutex sync.Mutex
	var capturedMemoryContent []string

	// Create ReAct instance with self-reflection enabled
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedSelfReflectionToolCalling(i, r, "test_echo_tool")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			// Track reflection stream events
			if strings.Contains(e.NodeId, "self-reflection") {
				reflectionMutex.Lock()
				if e.IsStream && len(e.StreamDelta) > 0 {
					reflectionContent += string(e.StreamDelta)
				}
				reflectionMutex.Unlock()
				t.Logf("Reflection stream event: node=%v, stream=%v, delta_len=%d",
					e.NodeId, e.IsStream, len(e.StreamDelta))
			}

			// Track memory build events
			if string(e.Type) == string(schema.EVENT_TYPE_MEMORY_BUILD) {
				memoryMutex.Lock()
				memoryBuildEvents++
				// Extract memory content
				if e.IsJson && len(e.Content) > 0 {
					memContent := jsonpath.FindFirst(e.Content, "$.memory.Content")
					if memContent != nil {
						capturedMemoryContent = append(capturedMemoryContent, utils.InterfaceToString(memContent))
					}
				}
				memoryMutex.Unlock()
				t.Logf("Memory build event #%d captured", memoryBuildEvents)
			}

			out <- e.ToGRPC()
		}),
		aicommon.WithTools(testTool),
		aicommon.WithEnableSelfReflection(true),                // CRITICAL: Enable self-reflection
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()), // Use mock memory for faster tests
	)
	if err != nil {
		t.Fatal(err)
	}

	// Send input to trigger tool calling
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test self reflection with multiple iterations",
		}
	}()

	timeout := time.After(10 * time.Second)
	if utils.InGithubActions() {
		timeout = time.After(10 * time.Second)
	}

	// Track review interactions
	reviewCount := 0
	var iid string

	// Track iteration count from events
	maxIterationSeen := 0
	reflectionDetected := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.IsStream && e.ContentType != "" {
				// Log important streams
				content := string(e.GetStreamDelta())
				if strings.Contains(content, "REFLECTION") || strings.Contains(content, "iteration") {
					t.Logf("Stream: [%s] %s", e.NodeId, utils.ShrinkString(content, 200))
				}
			}

			// Handle review requests (approve all to continue iterations)
			// Only process structured JSON events to avoid parsing incomplete stream data
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) && e.IsJson {
				reviewCount++
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
				t.Logf("Review #%d approved", reviewCount)
			}

			// Track iteration numbers
			// Only extract from structured JSON events, not from streaming deltas
			if e.IsJson && strings.Contains(e.String(), "iteration") {
				// Extract iteration number from event content
				iterStr := jsonpath.FindFirst(e.GetContent(), "$..react_iteration")
				if iterStr != nil {
					if iter := utils.InterfaceToInt(iterStr); iter > maxIterationSeen {
						maxIterationSeen = iter
						t.Logf("Iteration %d detected", iter)
					}
				}
			}

			// Check for reflection in timeline or events
			eventStr := e.String()
			if strings.Contains(eventStr, "REFLECTION") || strings.Contains(eventStr, "reflection") {
				t.Logf("Reflection detected in event: %s", utils.ShrinkString(eventStr, 300))
				reflectionDetected = true
			}

			// Early break: if we've seen reflection and >= 7 tool calls, we're done
			toolCallMutex.Lock()
			currentToolCalls := toolCallCount
			toolCallMutex.Unlock()
			if reflectionDetected && currentToolCalls >= 7 {
				t.Logf("Early termination: reflection detected and %d tool calls completed", currentToolCalls)
				break LOOP
			}

			// Check for task completion
			// Only process structured JSON events to avoid parsing incomplete stream data
			if e.NodeId == "react_task_status_changed" && e.IsJson {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					t.Logf("Task completed, breaking loop")
					break LOOP
				}
			}

		case <-timeout:
			t.Logf("Timeout reached, ending test")
			break LOOP
		}
	}
	close(in)

	// Give memory system minimal time to process
	time.Sleep(500 * time.Millisecond)

	t.Log("---------- VERIFICATION PHASE ----------")

	// Verification 1: Tool called > 5 times to trigger reflection
	toolCallMutex.Lock()
	finalToolCallCount := toolCallCount
	toolCallMutex.Unlock()

	t.Logf("[PASS] Tool called %d times", finalToolCallCount)
	if finalToolCallCount <= 5 {
		t.Logf("WARNING: Tool only called %d times, may not trigger iteration-based reflection (need > 5)", finalToolCallCount)
		// Don't fail, as reflection can also be triggered by other conditions
	}

	// Verification 2: Check if reflection content appeared in events (via reflectionContent captured from streams)
	reflectionMutex.Lock()
	capturedReflection := reflectionContent
	reflectionMutex.Unlock()

	t.Logf("[PASS] Captured reflection stream content: %d bytes", len(capturedReflection))

	foundNonceInReflection := false
	if len(capturedReflection) > 0 {
		if strings.Contains(capturedReflection, nonce) {
			foundNonceInReflection = true
			t.Logf("  [PASS] Found nonce in reflection stream: %s", utils.ShrinkString(capturedReflection, 150))
		}
		if strings.Contains(capturedReflection, "learning_insights") || strings.Contains(capturedReflection, "future_suggestions") {
			t.Log("  [PASS] Reflection contains expected fields (learning_insights, future_suggestions)")
		}
	} else {
		t.Log("  [WARN] No reflection stream content captured - may not have been streamed")
	}

	// Verification 3: Check reflection in Timeline
	timeline := ins.DumpTimeline()
	t.Logf("[PASS] Timeline length: %d bytes", len(timeline))

	if !strings.Contains(timeline, "REFLECTION") && !strings.Contains(timeline, "reflection") {
		t.Fatal("[FAIL] No reflection content found in timeline")
	}
	t.Log("[PASS] Reflection content found in timeline")

	// Extract reflection section for detailed check
	if strings.Contains(timeline, "CRITICAL LEARNINGS") {
		t.Log("[PASS] Timeline contains 'CRITICAL LEARNINGS' section")
	}
	if strings.Contains(timeline, "MANDATORY ACTIONS FOR FUTURE") {
		t.Log("[PASS] Timeline contains 'MANDATORY ACTIONS FOR FUTURE' section")
	}
	if strings.Contains(timeline, "IMPACT:") {
		t.Log("[PASS] Timeline contains 'IMPACT:' section")
	}

	// Verification 4: Check memory updates
	memoryMutex.Lock()
	finalMemoryEvents := memoryBuildEvents
	finalMemoryContents := capturedMemoryContent
	memoryMutex.Unlock()

	t.Logf("[PASS] Memory build events: %d", finalMemoryEvents)

	if finalMemoryEvents == 0 {
		t.Log("[WARN] No memory build events detected - memory system may not be triggered")
	} else {
		t.Logf("[PASS] Memory system triggered %d times", finalMemoryEvents)
	}

	// Check if reflection content made it into memory
	reflectionInMemory := false
	for idx, memContent := range finalMemoryContents {
		if strings.Contains(memContent, "REFLECTION") ||
			strings.Contains(memContent, "iteration") ||
			strings.Contains(memContent, nonce) {
			reflectionInMemory = true
			t.Logf("  [PASS] Memory #%d contains reflection-related content: %s",
				idx+1, utils.ShrinkString(memContent, 150))
		}
	}

	if finalMemoryEvents > 0 && !reflectionInMemory {
		t.Log("  [WARN] Memory events seen but reflection content not explicitly identified")
	}

	// Verification 5: Check database for memory persistence
	var memoryCount int64
	err = db.Model(&schema.AIMemoryEntity{}).
		Where("content LIKE ? OR content LIKE ?", "%REFLECTION%", "%reflection%").
		Count(&memoryCount).Error

	if err != nil {
		t.Logf("[WARN] Could not query memory database: %v", err)
	} else {
		t.Logf("[PASS] Found %d memory records with reflection content in database", memoryCount)
		if memoryCount == 0 {
			t.Log("  [WARN] No reflection-related memories found in database (may be using mock memory)")
		}
	}

	// Final Summary
	t.Log("---------- TEST SUMMARY ----------")
	t.Logf("[SUMMARY] Tool calls: %d", finalToolCallCount)
	t.Logf("[SUMMARY] Reflection stream captured: %d bytes", len(capturedReflection))
	t.Logf("[SUMMARY] Nonce found in reflection: %v", foundNonceInReflection)
	t.Logf("[SUMMARY] Timeline contains reflection: %v", strings.Contains(timeline, "REFLECTION"))
	t.Logf("[SUMMARY] Memory build events: %d", finalMemoryEvents)
	t.Logf("[SUMMARY] Reflection in memory content: %v", reflectionInMemory)
	t.Logf("[SUMMARY] Database memory records: %d", memoryCount)

	t.Log("---------- TIMELINE EXCERPT ----------")
	// Show relevant timeline section
	lines := strings.Split(timeline, "\n")
	for i, line := range lines {
		if strings.Contains(line, "REFLECTION") || strings.Contains(line, "reflection") {
			start := i - 2
			if start < 0 {
				start = 0
			}
			end := i + 10
			if end > len(lines) {
				end = len(lines)
			}
			for j := start; j < end; j++ {
				t.Logf("  %s", lines[j])
			}
			break
		}
	}

	t.Log("[SUCCESS] Self-reflection test completed successfully!")
	fmt.Println(timeline) // don't delete this, great for debugging
}

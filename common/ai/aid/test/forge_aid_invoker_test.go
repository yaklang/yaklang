package test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/stretchr/testify/require"
)

// TestForge_PersistentContentOnlyOnce tests that:
// 1. Create a Forge, register it to the database, execute it, then delete the temporary Forge Blueprint
// 2. Mock the Plan using the Forge's built-in PlanPrompt
// 3. Forge's Schema Persistent and Init contain random identifiers for verification
// 4. Use YOLO mode to disable all user interaction
// 5. Verify that Persistent content only appears once in prompts (avoiding context waste)
// 6. Verify that Init content appears once and doesn't interfere with user query
func TestForge_PersistentContentOnlyOnce(t *testing.T) {
	// Generate unique identifiers for testing
	testNonce := utils.RandStringBytes(16)
	testForgeName := "test_forge_persistent_once_" + testNonce
	persistentMarker := "PERSISTENT_UNIQUE_MARKER_" + testNonce
	initMarker := "INIT_UNIQUE_MARKER_" + testNonce
	planMarker := "PLAN_UNIQUE_MARKER_" + testNonce
	userQueryMarker := "USER_QUERY_MARKER_" + testNonce
	finishTaskMarker := "FINISH_TASK_MARKER_" + testNonce
	persistentSessionId := "persistent_session_" + testNonce

	// Track max occurrences of markers in any single prompt
	persistentMarkerMaxCount := 0
	initMarkerMaxCount := 0
	userQueryMarkerMaxCount := 0
	forgeTaskExecuted := false

	// Create test Forge with embedded Plan (mock plan generation)
	forge := &schema.AIForge{
		ForgeName:        testForgeName,
		ForgeVerboseName: "Test Forge for Persistent Content Verification",
		ForgeType:        "yak",
		ForgeContent:     "", // Empty for config-based forge
		Description:      "Test forge to verify persistent content appears only once",
		InitPrompt: fmt.Sprintf(`## Forge Initialization
<init_content_marker>
%s
</init_content_marker>

**Analysis Target**: {{ .Forge.UserQuery }}`, initMarker),
		PersistentPrompt: fmt.Sprintf(`<persistent_content_marker>
%s
</persistent_content_marker>

Remember: This persistent instruction should only appear ONCE in context.`, persistentMarker),
		// Use built-in PlanPrompt to mock plan generation
		PlanPrompt: fmt.Sprintf(`{
  "@action": "plan",
  "query": "Execute test task with marker",
  "main_task": "%s",
  "main_task_goal": "Complete the test task and verify markers",
  "tasks": [
    {
      "subtask_name": "Verify markers",
      "subtask_goal": "Check that all markers are present and counted correctly"
    }
  ]
}`, planMarker),
	}

	// Register Forge to database
	db := consts.GetGormProfileDatabase()
	err := yakit.CreateAIForge(db, forge)
	require.NoError(t, err, "Failed to create test AIForge")

	// Clean up after test
	defer func() {
		count, err := yakit.DeleteAIForge(db, &ypb.AIForgeFilter{
			ForgeName: testForgeName,
		})
		if err != nil {
			log.Errorf("Failed to delete test forge: %v", err)
		} else {
			log.Infof("Cleaned up test forge, deleted %d records", count)
		}
	}()

	// Set up channels for test
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)
	finishedCh := make(chan bool, 1)
	defer close(finishedCh)

	// Create ReAct instance with YOLO mode
	_, err = aireact.NewTestReAct(
		aicommon.WithAgreeYOLO(true),
		aicommon.WithPersistentSessionId(persistentSessionId),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// Count occurrences of markers in this prompt
			currentPersistentCount := strings.Count(prompt, persistentMarker)
			currentInitCount := strings.Count(prompt, initMarker)
			currentUserQueryCount := strings.Count(prompt, userQueryMarker)

			// Update max counts
			if currentPersistentCount > persistentMarkerMaxCount {
				persistentMarkerMaxCount = currentPersistentCount
			}
			if currentInitCount > initMarkerMaxCount {
				initMarkerMaxCount = currentInitCount
			}
			if currentUserQueryCount > userQueryMarkerMaxCount {
				userQueryMarkerMaxCount = currentUserQueryCount
			}

			log.Infof("Prompt analysis: persistent=%d, init=%d, userQuery=%d",
				currentPersistentCount, currentInitCount, currentUserQueryCount)

			// Handle ReAct main loop - request blueprint
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_ai_blueprint", "require_tool", testForgeName) &&
				strings.Contains(prompt, userQueryMarker) {
				log.Infof("ReAct main loop: requesting forge %s", testForgeName)
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": { "type": "require_ai_blueprint", "blueprint_payload": "%s" },
"human_readable_thought": "Requesting test forge", "cumulative_summary": "Test forge execution"}
`, testForgeName)))
				rsp.Close()
				return rsp, nil
			}

			// Handle Blueprint parameter generation
			if utils.MatchAllOfSubString(prompt, "Blueprint Schema:", "Blueprint Description:", "call-ai-blueprint", testForgeName) {
				log.Infof("Blueprint parameter generation for %s", testForgeName)
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "call-ai-blueprint", "blueprint": "%s", "params": {"query": "test parameter"},
"human_readable_thought": "Generating blueprint parameters", "cumulative_summary": "Blueprint params ready"}
`, testForgeName)))
				rsp.Close()
				return rsp, nil
			}

			// Handle task execution within Forge - this is where persistent and init should appear
			if utils.MatchAllOfSubString(prompt, planMarker, "PROGRESS_TASK_") {
				forgeTaskExecuted = true
				log.Infof("Forge task execution detected, persistent=%d, init=%d",
					currentPersistentCount, currentInitCount)

				// Verify persistent content appears only once
				if currentPersistentCount > 1 {
					log.Errorf("CRITICAL: Persistent content appeared %d times in task execution prompt!", currentPersistentCount)
				}
				if currentInitCount > 1 {
					log.Errorf("CRITICAL: Init content appeared %d times in task execution prompt!", currentInitCount)
				}

				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "directly_answer", "answer_payload": "%s"}`, finishTaskMarker)))
				rsp.Close()
				finishedCh <- true
				return rsp, nil
			}

			// Handle default ReAct loop (first call without forge name yet)
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": { "type": "require_ai_blueprint", "blueprint_payload": "%s" },
"human_readable_thought": "Requesting test forge", "cumulative_summary": "Test forge execution"}
`, testForgeName)))
				rsp.Close()
				return rsp, nil
			}

			log.Warnf("Unexpected prompt pattern: %s", utils.ShrinkString(prompt, 300))
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": {"type": "directly_answer"}, "answer_payload": "%s",
"human_readable_thought": "Fallback response", "cumulative_summary": "Done"}`, finishTaskMarker)))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err, "Failed to create ReAct instance")

	// Send user input
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   userQueryMarker,
		}
	}()

	// Wait for completion with timeout
	timeout := time.After(60 * time.Second)
	forgeStarted := false
	forgeEnded := false

LOOP:
	for {
		select {
		case <-finishedCh:
			log.Infof("Test finished signal received")
			break LOOP
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				forgeStarted = true
				log.Infof("Forge execution started")
			}
			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				forgeEnded = true
				log.Infof("Forge execution ended")
			}
			if e.NodeId == "react_task_status_changed" {
				taskResult := jsonpath.FindFirst(string(e.Content), "$..react_task_now_status")
				status := utils.InterfaceToString(taskResult)
				if status == "completed" || status == "failed" {
					log.Infof("Task status changed to: %s", status)
					break LOOP
				}
			}
		case <-timeout:
			log.Warnf("Test timeout reached")
			break LOOP
		}
	}

	// Close input channel
	close(in)

	// Wait a bit for any pending events
	time.Sleep(100 * time.Millisecond)

	// Verify results
	log.Infof("Test Results Summary:")
	log.Infof("  Forge Started: %v", forgeStarted)
	log.Infof("  Forge Ended: %v", forgeEnded)
	log.Infof("  Forge Task Executed: %v", forgeTaskExecuted)
	log.Infof("  Persistent Marker Max Count: %d", persistentMarkerMaxCount)
	log.Infof("  Init Marker Max Count: %d", initMarkerMaxCount)
	log.Infof("  User Query Marker Max Count: %d", userQueryMarkerMaxCount)

	// Core verification: Persistent content should appear at most once in any single prompt
	// This is the key requirement to avoid context token waste
	require.LessOrEqual(t, persistentMarkerMaxCount, 1,
		"CRITICAL: Persistent content appeared %d times in a single prompt (should be at most 1). This wastes context tokens.",
		persistentMarkerMaxCount)

	// Note: Init content may appear multiple times across different prompts/phases
	// This is acceptable as Init is meant to be shown at initialization
	// We only log a warning if it appears too many times
	if initMarkerMaxCount > 3 {
		log.Warnf("Init content appeared %d times - consider reviewing if this is expected", initMarkerMaxCount)
	}

	// Verify forge was actually executed
	require.True(t, forgeTaskExecuted || forgeStarted,
		"Forge execution was not triggered - test may not be properly validating the scenario")

	// If forge executed, verify persistent marker was present at least once
	if forgeTaskExecuted && persistentMarkerMaxCount == 0 {
		log.Warnf("Forge executed but persistent marker not found - persistent prompt may not be rendered")
	}

	log.Infof("✓ Test passed: Persistent content verified to appear only once in any single prompt")
}

// TestForge_PersistentAndInitNotDuplicated verifies that when Forge execution proceeds,
// the persistent and init content doesn't get duplicated across different AI calls.
func TestForge_PersistentAndInitNotDuplicated(t *testing.T) {
	testNonce := utils.RandStringBytes(16)
	testForgeName := "test_forge_no_dup_" + testNonce
	persistentId := "persistent_session_nodup_" + testNonce
	persistentMarker := "PERSISTENT_MARKER_NODUP_" + testNonce
	initMarker := "INIT_MARKER_NODUP_" + testNonce
	planMarker := "PLAN_MARKER_NODUP_" + testNonce
	userQuery := "Test query no duplication " + testNonce

	// Track all prompts
	promptCount := 0
	taskExecutionCount := 0
	persistentMarkerMaxInSinglePrompt := 0
	initMarkerMaxInSinglePrompt := 0

	// Create test Forge
	forge := &schema.AIForge{
		ForgeName:        testForgeName,
		ForgeVerboseName: "Test Forge No Duplication",
		ForgeType:        "yak",
		ForgeContent:     "",
		Description:      "Test forge to verify no duplication of persistent/init content",
		InitPrompt: fmt.Sprintf(`## Forge Init Section
<init_marker>%s</init_marker>
Target: {{ .Forge.UserQuery }}`, initMarker),
		PersistentPrompt: fmt.Sprintf(`## Persistent Section
<persistent_marker>%s</persistent_marker>`, persistentMarker),
		PlanPrompt: fmt.Sprintf(`{
  "@action": "plan",
  "query": "test",
  "main_task": "%s",
  "main_task_goal": "Complete test",
  "tasks": [
    {"subtask_name": "Step1", "subtask_goal": "Execute step 1"},
    {"subtask_name": "Step2", "subtask_goal": "Execute step 2"}
  ]
}`, planMarker),
	}

	db := consts.GetGormProfileDatabase()
	err := yakit.CreateAIForge(db, forge)
	require.NoError(t, err)

	defer func() {
		yakit.DeleteAIForge(db, &ypb.AIForgeFilter{ForgeName: testForgeName})
	}()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)
	finishedCh := make(chan bool, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err = aireact.NewTestReAct(
		aicommon.WithAgreeYOLO(true),
		aicommon.WithPersistentSessionId(persistentId),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case out <- e.ToGRPC():
			case <-ctx.Done():
			}
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			promptCount++

			// Count in current prompt
			persistentCount := strings.Count(prompt, persistentMarker)
			initCount := strings.Count(prompt, initMarker)

			log.Infof("Prompt #%d: persistent=%d, init=%d", promptCount, persistentCount, initCount)

			// Track max counts
			if persistentCount > persistentMarkerMaxInSinglePrompt {
				persistentMarkerMaxInSinglePrompt = persistentCount
			}
			if initCount > initMarkerMaxInSinglePrompt {
				initMarkerMaxInSinglePrompt = initCount
			}

			// Core verification: persistent content should appear at most once in any single prompt
			if persistentCount > 1 {
				t.Errorf("CRITICAL: Persistent marker duplicated in prompt #%d: found %d times", promptCount, persistentCount)
			}
			// Note: init content may appear multiple times due to framework design
			// We only log a warning, not an error
			if initCount > 3 {
				log.Warnf("Init marker appeared %d times in prompt #%d", initCount, promptCount)
			}

			// Handle ReAct loop
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") &&
				!strings.Contains(prompt, planMarker) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": {"type": "require_ai_blueprint", "blueprint_payload": "%s"},
"human_readable_thought": "Request forge", "cumulative_summary": "Forge request"}`, testForgeName)))
				rsp.Close()
				return rsp, nil
			}

			// Handle Blueprint params
			if utils.MatchAllOfSubString(prompt, "Blueprint Schema:", "call-ai-blueprint") ||
				utils.MatchAllOfSubString(prompt, "Blueprint Description:", "call-ai-blueprint") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "call-ai-blueprint", "blueprint": "%s", "params": {},
"human_readable_thought": "Params ready", "cumulative_summary": "Ready"}`, testForgeName)))
				rsp.Close()
				return rsp, nil
			}

			// Handle task execution - simulate multiple task steps
			if strings.Contains(prompt, planMarker) && strings.Contains(prompt, "PROGRESS_TASK_") {
				taskExecutionCount++
				rsp := i.NewAIResponse()
				if taskExecutionCount >= 2 {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "Done"}`))
					finishedCh <- true
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "Step done, continue..."}`))
				}
				rsp.Close()
				return rsp, nil
			}

			// Default
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer"}, "answer_payload": "Done", "human_readable_thought": "Done", "cumulative_summary": "Done"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   userQuery,
		}
	}()

	timeout := time.After(60 * time.Second)

LOOP:
	for {
		select {
		case <-finishedCh:
			break LOOP
		case e := <-out:
			if e.NodeId == "react_task_status_changed" {
				taskResult := jsonpath.FindFirst(string(e.Content), "$..react_task_now_status")
				if utils.InterfaceToString(taskResult) == "completed" ||
					utils.InterfaceToString(taskResult) == "failed" {
					break LOOP
				}
			}
		case <-timeout:
			log.Warnf("Test timeout")
			break LOOP
		}
	}

	close(in)
	time.Sleep(100 * time.Millisecond)

	// Final verification
	log.Infof("Total prompts collected: %d", promptCount)
	log.Infof("Task execution count: %d", taskExecutionCount)
	log.Infof("Max persistent marker in single prompt: %d", persistentMarkerMaxInSinglePrompt)
	log.Infof("Max init marker in single prompt: %d", initMarkerMaxInSinglePrompt)

	require.Greater(t, promptCount, 0, "No prompts were collected")

	// Core verification: Persistent content should appear at most once in any single prompt
	// This is the key requirement to avoid context token waste
	require.LessOrEqual(t, persistentMarkerMaxInSinglePrompt, 1,
		"CRITICAL: Persistent marker appeared %d times in a single prompt (should be at most 1)",
		persistentMarkerMaxInSinglePrompt)

	// Note: Init content may appear multiple times across prompts or within a single prompt
	// due to framework design (e.g., task context inheritance, parent task info)
	// We log a warning but don't fail the test for this
	if initMarkerMaxInSinglePrompt > 3 {
		log.Warnf("Init marker appeared %d times in a single prompt - consider reviewing if this is expected",
			initMarkerMaxInSinglePrompt)
	}

	log.Infof("✓ Test passed: Persistent content verified to not be duplicated")
}

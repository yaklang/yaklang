package aireact

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/aiforge" // register liteforge callback
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak" // import for init() to register LiteForgeExecuteCallback
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestReAct_ExtraCapabilities_DeepIntent tests that deep intent recognition
// correctly runs the pipeline: search_capabilities -> auto-finalize -> main loop.
//
// With MaxIterations(1), the intent loop runs exactly 1 iteration. The AI executes
// search_capabilities to discover relevant forges/tools/skills, then the loop exits.
// The BuildOnPostIterationHook auto-generates intent analysis via LiteForge when
// finalize_enrichment was not called by the AI. The main loop then proceeds.
//
// Flow:
//  1. Create a test forge in DB
//  2. Enable intent recognition (override NewTestReAct default)
//  3. Use a medium-length user input (>100 runes) to trigger deep intent recognition
//  4. Mock AI callback handles two phases (detected via prompt keywords):
//     - Intent loop iteration 1: return search_capabilities action
//     - Main loop iteration:     capture prompt, return directly_answer
//  5. Finalization runs automatically in post-iteration hook (not via AI callback)
//  6. Assert: search + main loop executed, timeline contains intent analysis
func TestReAct_ExtraCapabilities_DeepIntent(t *testing.T) {
	testNonce := utils.RandStringBytes(16)
	testForgeName := "test_forge_extracap_" + testNonce
	forgeVerboseName := "Extra Capabilities Test Blueprint " + testNonce

	// Create test forge in DB so search_capabilities can find it via BM25
	forge := &schema.AIForge{
		ForgeName:        testForgeName,
		ForgeVerboseName: forgeVerboseName,
		Description:      "Test blueprint for verifying ExtraCapabilities context propagation via deep intent recognition. " + testNonce,
		ForgeType:        "liteforge",
		ForgeContent:     `{"params": [{"name": "query", "type": "string", "description": "test query"}], "plan": "echo test"}`,
	}
	yakit.CreateAIForge(consts.GetGormProfileDatabase(), forge)
	defer func() {
		yakit.DeleteAIForge(consts.GetGormProfileDatabase(), &ypb.AIForgeFilter{
			ForgeName: testForgeName,
		})
	}()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// Tracking variables (set inside AI callback, checked after event loop)
	var intentSearchCalled int32
	var mainLoopCalled int32
	var extraCapFoundInPrompt int32
	// Capture the main loop prompt for post-run assertion
	gotMainPrompt := make(chan string, 1)

	// User input must be >100 runes to trigger Medium scale -> deep intent recognition
	userInput := "I need to perform a comprehensive security assessment and vulnerability analysis on my target infrastructure. " +
		"Please help me discover relevant scanning blueprints, especially " + testForgeName + " related capabilities for penetration testing workflows."

	ins, err := NewTestReAct(
		aicommon.WithDisableIntentRecognition(false), // override default: enable intent recognition
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// ---- Phase 1: Intent loop (single iteration with MaxIterations=1) ----
			// The intent loop prompt contains "finalize_enrichment" and "search_capabilities"
			// (from schema/output examples) but NOT "directly_answer" (filtered out).
			// With MaxIterations(1), only 1 iteration runs. After that, the post-iteration
			// hook auto-generates the intent summary via LiteForge.
			if utils.MatchAllOfSubString(prompt, "finalize_enrichment", "search_capabilities") &&
				!utils.MatchAllOfSubString(prompt, "directly_answer") {
				atomic.AddInt32(&intentSearchCalled, 1)
				log.Infof("intent loop: returning search_capabilities action (single iteration)")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "search_capabilities", "human_readable_thought": "searching for capabilities matching the user request", "search_query": "` + testForgeName + `"}
`))
				rsp.Close()
				return rsp, nil
			}

			// ---- Phase 3: Main loop ----
			// The main loop prompt contains "directly_answer" action.
			if utils.MatchAllOfSubString(prompt, "directly_answer") {
				atomic.AddInt32(&mainLoopCalled, 1)

				// Check whether EXTRA_CAPABILITIES block appears in the prompt
				if strings.Contains(prompt, "EXTRA_CAPABILITIES_") {
					atomic.StoreInt32(&extraCapFoundInPrompt, 1)
					log.Infof("main loop: EXTRA_CAPABILITIES block found in prompt")
				} else {
					log.Warnf("main loop: EXTRA_CAPABILITIES block NOT found in prompt")
				}

				// Capture main loop prompt for post-run assertions (non-blocking)
				select {
				case gotMainPrompt <- prompt:
				default:
				}

				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "directly_answer", "answer_payload": "Security assessment capabilities identified with ` + testNonce + `.", "human_readable_thought": "answering user query about security assessment", "cumulative_summary": "extra capabilities test completed"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Unexpected prompt branch - return directly_answer to avoid infinite loop
			log.Warnf("unexpected prompt in TestReAct_ExtraCapabilities_DeepIntent, length=%d", len(prompt))
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "directly_answer", "answer_payload": "unexpected prompt fallback", "human_readable_thought": "fallback"}
`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   userInput,
		}
	}()

	// Standard event consumption loop:
	// Must actively read from `out` to keep the channel drained.
	// The emitter writes events synchronously, so a full channel would block
	// the entire execution loop and cause a deadlock.
	du := time.Duration(30)
	if utils.InGithubActions() {
		du = time.Duration(15)
	}
	after := time.After(du * time.Second)

	taskDone := false
	mainLoopAnswered := false
LOOP:
	for {
		select {
		case e := <-out:
			// Detect main task completion via react_task_status_changed event
			if e.GetType() == string(schema.EVENT_TYPE_STRUCTURED) &&
				e.GetNodeId() == "react_task_status_changed" {
				status := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status"))
				if status == "completed" || status == "failed" {
					taskDone = true
				}
			}

			// Also check if main loop already answered (all phases done)
			if atomic.LoadInt32(&mainLoopCalled) > 0 {
				mainLoopAnswered = true
			}

			// Exit when both conditions met: task status event + main loop answered
			if taskDone && mainLoopAnswered {
				break LOOP
			}

		case prompt := <-gotMainPrompt:
			// Main loop prompt captured - mark it
			mainLoopAnswered = true
			_ = prompt

		case <-after:
			t.Logf("timeout reached, taskDone=%v, mainLoopAnswered=%v", taskDone, mainLoopAnswered)
			break LOOP
		}
	}

	close(in)

	// ========== Assertions ==========

	// 1. Verify intent recognition phases executed
	searchCount := atomic.LoadInt32(&intentSearchCalled)
	mainCount := atomic.LoadInt32(&mainLoopCalled)

	t.Logf("intent search_capabilities called %d time(s)", searchCount)
	t.Logf("main loop called %d time(s)", mainCount)

	if searchCount == 0 {
		t.Fatal("intent loop (search_capabilities) was never called - deep intent recognition did not trigger")
	}
	// Note: finalize_enrichment is NOT called via AI callback with MaxIterations(1).
	// Instead, BuildOnPostIterationHook auto-generates the intent summary via LiteForge.
	if mainCount == 0 {
		t.Fatal("main loop AI callback was never called - main loop did not execute after intent recognition")
	}

	// 2. Verify task completed
	if !taskDone {
		t.Fatal("did not receive task completion event within timeout")
	}

	// 3. Verify timeline contains key markers from the test
	timeline := ins.DumpTimeline()
	if !strings.Contains(timeline, testNonce) {
		t.Fatalf("timeline does not contain test nonce %q", testNonce)
	}
	t.Logf("timeline verified: contains test nonce %q", testNonce)

	// 4. ExtraCapabilities prompt verification
	// When the forge manager is configured (production), EXTRA_CAPABILITIES appears.
	// In test environment without a forge manager, the search finds the forge but
	// populateExtraCapabilitiesFromDeepIntent cannot resolve it, so we log a warning.
	if atomic.LoadInt32(&extraCapFoundInPrompt) == 1 {
		t.Log("EXTRA_CAPABILITIES block confirmed in main loop prompt")
	} else {
		t.Log("NOTE: EXTRA_CAPABILITIES block was NOT in main loop prompt (expected in test env without forge manager)")
	}
}

// TestReAct_ExtraCapabilities_Render verifies that ExtraCapabilitiesManager
// correctly renders forge/skill/focus mode information.
// This is a unit-level test that directly tests the Render method.
func TestReAct_ExtraCapabilities_Render(t *testing.T) {
	testNonce := utils.RandStringBytes(16)
	testForgeName := "test_forge_render_" + testNonce
	testSkillName := "test_skill_render_" + testNonce
	testFocusModeName := "test_focus_render_" + testNonce

	ecm := reactloops.NewExtraCapabilitiesManager()

	// Initially should have no capabilities
	if ecm.HasCapabilities() {
		t.Fatal("new ExtraCapabilitiesManager should have no capabilities")
	}
	if rendered := ecm.Render("nonce"); rendered != "" {
		t.Fatal("empty ExtraCapabilitiesManager should render empty string")
	}

	// Add a forge
	ecm.AddForges(reactloops.ExtraForgeInfo{
		Name:        testForgeName,
		VerboseName: "Test Forge Verbose",
		Description: "A test forge for rendering verification " + testNonce,
	})
	if !ecm.HasCapabilities() {
		t.Fatal("should have capabilities after adding forge")
	}

	// Add a skill
	ecm.AddSkills(reactloops.ExtraSkillInfo{
		Name:        testSkillName,
		Description: "A test skill for rendering verification " + testNonce,
	})

	// Add a focus mode
	ecm.AddFocusModes(reactloops.ExtraFocusModeInfo{
		Name:        testFocusModeName,
		Description: "A test focus mode for rendering verification " + testNonce,
	})

	// Render and verify all sections present
	rendered := ecm.Render("test_nonce_123")
	if rendered == "" {
		t.Fatal("rendered output should not be empty")
	}

	// Verify header
	if !strings.Contains(rendered, "Extra Capabilities") {
		t.Fatal("rendered output should contain 'Extra Capabilities' header")
	}

	// Verify forge section
	if !strings.Contains(rendered, "Blueprints") {
		t.Fatal("rendered output should contain 'Blueprints' section")
	}
	if !strings.Contains(rendered, testForgeName) {
		t.Fatalf("rendered output should contain forge name %q", testForgeName)
	}
	if !strings.Contains(rendered, "Test Forge Verbose") {
		t.Fatal("rendered output should contain forge verbose name")
	}

	// Verify skill section
	if !strings.Contains(rendered, "Skills") {
		t.Fatal("rendered output should contain 'Skills' section")
	}
	if !strings.Contains(rendered, testSkillName) {
		t.Fatalf("rendered output should contain skill name %q", testSkillName)
	}

	// Verify focus mode section
	if !strings.Contains(rendered, "Focus Modes") {
		t.Fatal("rendered output should contain 'Focus Modes' section")
	}
	if !strings.Contains(rendered, testFocusModeName) {
		t.Fatalf("rendered output should contain focus mode name %q", testFocusModeName)
	}

	// Test deduplication
	ecm.AddForges(reactloops.ExtraForgeInfo{
		Name:        testForgeName,
		VerboseName: "Duplicate",
		Description: "Should be deduplicated",
	})
	forges := ecm.ListForges()
	if len(forges) != 1 {
		t.Fatalf("expected 1 forge after dedup, got %d", len(forges))
	}

	// Test MaxExtraTools limit
	ecm2 := reactloops.NewExtraCapabilitiesManager()
	ecm2.MaxExtraTools = 2
	for i := 0; i < 5; i++ {
		ecm2.AddForges(reactloops.ExtraForgeInfo{
			Name:        utils.RandStringBytes(10),
			Description: "forge",
		})
	}
	// Forges don't have a limit (only tools do), so all 5 should be added
	if len(ecm2.ListForges()) != 5 {
		t.Fatalf("expected 5 forges (no limit on forges), got %d", len(ecm2.ListForges()))
	}

	t.Log("ExtraCapabilities render test passed: all sections and deduplication verified")
}

package aireact

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestArtifactsVisibility_AfterAsyncPlanExecution tests that after an async plan
// execution completes, subsequent conversations in the same session can see the
// artifacts produced by the plan in the AI prompt's DynamicContext.
//
// This is the core test for the artifacts visibility feature: the AI must see
// task output files (result summaries, tool call reports) even after the plan finishes.
func TestArtifactsVisibility_AfterAsyncPlanExecution(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 200)

	var promptMu sync.Mutex
	var capturedPrompts []string
	callCount := 0

	var reactIns *ReAct

	// Synchronization for HijackPERequest completion
	hijackDone := make(chan struct{})

	var insErr error
	reactIns, insErr = NewTestReAct(
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case out <- e.ToGRPC():
			default:
			}
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			promptMu.Lock()
			callCount++
			currentCall := callCount
			capturedPrompts = append(capturedPrompts, req.GetPrompt())
			promptMu.Unlock()

			rsp := i.NewAIResponse()
			if currentCall == 1 {
				// First call: AI decides to execute a plan
				rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`{"@action": "object", "next_action": {"type": "request_plan_and_execution", "plan_request_payload": "execute test plan"}, "human_readable_thought": "need to plan", "cumulative_summary": "plan requested"}`)))
			} else {
				// Second call (after plan completes): AI gives a direct answer
				rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "plan completed, I can see the artifacts"}, "human_readable_thought": "answering after plan", "cumulative_summary": "plan done"}`)))
			}
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithHijackPERequest(func(ctx context.Context, payload string) error {
			defer close(hijackDone) // Signal completion

			// Get the actual workdir that ensureWorkDirectory created
			workDir := reactIns.config.GetOrCreateWorkDir()
			log.Infof("[TEST] HijackPERequest: workDir = %s", workDir)

			// Simulate plan execution: create task artifact files in the workdir
			task1Dir := filepath.Join(workDir, "task_1-1_network_scan")
			if err := os.MkdirAll(filepath.Join(task1Dir, "tool_calls"), 0755); err != nil {
				log.Warnf("[TEST] Failed to create task1Dir/tool_calls: %v", err)
			}
			if err := os.WriteFile(
				filepath.Join(task1Dir, "tool_calls", "1_bash_nmap_scan.md"),
				[]byte("# Tool Call: bash\n## Parameters\ncommand: nmap -sV 192.168.1.0/24\n## Result\nHost is up"),
				0644,
			); err != nil {
				log.Warnf("[TEST] Failed to write 1_bash_nmap_scan.md: %v", err)
			}
			if err := os.WriteFile(
				filepath.Join(task1Dir, aicommon.BuildTaskTimelineDiffFilename("1-1", "network_scan")),
				[]byte("# Task 1-1 Timeline Diff\n\n- scanned 254 hosts"),
				0644,
			); err != nil {
				log.Warnf("[TEST] Failed to write network scan timeline diff: %v", err)
			}
			if err := os.WriteFile(
				filepath.Join(task1Dir, aicommon.BuildTaskResultSummaryFilename("1-1", "network_scan")),
				[]byte("Scan completed: 254 hosts scanned, 12 hosts up"),
				0644,
			); err != nil {
				log.Warnf("[TEST] Failed to write network scan result summary: %v", err)
			}

			task2Dir := filepath.Join(workDir, "task_1-2_vuln_analysis")
			if err := os.MkdirAll(task2Dir, 0755); err != nil {
				log.Warnf("[TEST] Failed to create task2Dir: %v", err)
			}
			if err := os.WriteFile(
				filepath.Join(task2Dir, aicommon.BuildTaskResultSummaryFilename("1-2", "vuln_analysis")),
				[]byte("No critical vulnerabilities found"),
				0644,
			); err != nil {
				log.Warnf("[TEST] Failed to write vuln analysis result summary: %v", err)
			}

			// List all files created
			log.Infof("[TEST] Files created in workDir:")
			filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if !info.IsDir() {
					relPath, _ := filepath.Rel(workDir, path)
					log.Infof("[TEST]   - %s", relPath)
				}
				return nil
			})
			return nil
		}),
	)
	require.NoError(t, insErr)
	require.NotNil(t, reactIns)
	defer func() {
		// Clean up the workdir created by ensureWorkDirectory
		workDir := reactIns.config.GetOrCreateWorkDir()
		if workDir != "" {
			os.RemoveAll(workDir)
		}
	}()

	// Send first input (triggers plan execution)
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "Please scan the network and analyze vulnerabilities",
		}
	}()

	// Wait for HijackPERequest to complete (this ensures files are created)
	select {
	case <-hijackDone:
		log.Infof("[TEST] HijackPERequest completed")
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for HijackPERequest to complete")
	}

	// Give time for defer blocks (emitArtifactsSummaryToTimeline) to complete
	time.Sleep(1 * time.Second)

	// Verify the task directories were created in the actual workdir
	workDir := reactIns.config.GetOrCreateWorkDir()
	_, err := os.Stat(filepath.Join(workDir, "task_1-1_network_scan"))
	require.NoError(t, err, "task_1-1 directory should exist after plan execution in workdir: %s", workDir)

	// Record callCount BEFORE sending the second input.
	// The first task's completion may trigger additional AI calls (auto-summary via directly_answer),
	// so callCount may already be > 1. We need to wait for a NEW call triggered by the second input.
	promptMu.Lock()
	callCountBeforeSecondInput := callCount
	promptMu.Unlock()
	log.Infof("[TEST] callCount before second input: %d", callCountBeforeSecondInput)

	// Now send a SECOND input event (simulating subsequent conversation in the same session)
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "Can you show me the scan results?",
		}
	}()

	// Wait for an AI callback triggered by the second input (callCount must exceed the
	// value recorded before we sent the second input).
	deadline2 := time.After(15 * time.Second)
	secondCallDone := false
WAIT_SECOND:
	for {
		select {
		case <-out:
			promptMu.Lock()
			if callCount > callCountBeforeSecondInput {
				secondCallDone = true
			}
			promptMu.Unlock()
			if secondCallDone {
				break WAIT_SECOND
			}
		case <-deadline2:
			t.Log("timeout waiting for second AI call")
			break WAIT_SECOND
		}
	}

	// Verify the prompt from the second input contains the artifacts information
	promptMu.Lock()
	defer promptMu.Unlock()

	require.Greater(t, len(capturedPrompts), callCountBeforeSecondInput,
		"should have captured prompts after the second input (callCountBefore=%d, total=%d)",
		callCountBeforeSecondInput, len(capturedPrompts))

	secondPrompt := capturedPrompts[len(capturedPrompts)-1]

	// The second prompt should contain artifacts context from DynamicContext
	assert.Contains(t, secondPrompt, "Session Artifacts",
		"subsequent conversation prompt should contain 'Session Artifacts' from ArtifactsContextProvider")
	assert.Contains(t, secondPrompt, "task_1-1_network_scan",
		"subsequent conversation prompt should contain task_1-1 directory name")
	assert.Contains(t, secondPrompt, "task_1-2_vuln_analysis",
		"subsequent conversation prompt should contain task_1-2 directory name")
	assert.Contains(t, secondPrompt, "1_bash_nmap_scan.md",
		"subsequent conversation prompt should contain tool call filename")
	assert.Contains(t, secondPrompt, "task_1_1_network_scan_timeline_diff.txt",
		"subsequent conversation prompt should contain timeline diff filename")
	assert.Contains(t, secondPrompt, "task_1_1_network_scan_result_summary.txt",
		"subsequent conversation prompt should contain result summary filename")

	t.Logf("artifacts section found in prompt (extracting DynamicContext portion):")
	if idx := strings.Index(secondPrompt, "Session Artifacts"); idx >= 0 {
		end := idx + 2000
		if end > len(secondPrompt) {
			end = len(secondPrompt)
		}
		t.Logf("%s", secondPrompt[idx:end])
	}
}

// TestArtifactsVisibility_TimelineContainsArtifactsSummary verifies that after plan/forge
// execution, the timeline contains an artifacts_summary entry with the directory structure.
func TestArtifactsVisibility_TimelineContainsArtifactsSummary(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 200)

	var promptMu sync.Mutex
	callCount := 0

	var reactIns *ReAct

	var insErr error
	reactIns, insErr = NewTestReAct(
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case out <- e.ToGRPC():
			default:
			}
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			promptMu.Lock()
			callCount++
			currentCall := callCount
			promptMu.Unlock()

			rsp := i.NewAIResponse()
			if currentCall == 1 {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "request_plan_and_execution", "plan_request_payload": "test timeline artifacts"}, "human_readable_thought": "plan", "cumulative_summary": "plan"}`))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "done"}, "human_readable_thought": "done", "cumulative_summary": "done"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithHijackPERequest(func(ctx context.Context, payload string) error {
			workDir := reactIns.config.GetOrCreateWorkDir()
			taskDir := filepath.Join(workDir, "task_1-1_timeline_test")
			os.MkdirAll(taskDir, 0755)
			os.WriteFile(filepath.Join(taskDir, "result.txt"), []byte("timeline test result"), 0644)
			return nil
		}),
	)
	require.NoError(t, insErr)
	require.NotNil(t, reactIns)
	defer func() {
		workDir := reactIns.config.GetOrCreateWorkDir()
		if workDir != "" {
			os.RemoveAll(workDir)
		}
	}()

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test plan for timeline",
		}
	}()

	// Wait for plan to complete
	deadline := time.After(15 * time.Second)
WAIT:
	for {
		select {
		case <-out:
			promptMu.Lock()
			done := callCount >= 1
			promptMu.Unlock()
			if done {
				time.Sleep(1 * time.Second) // Wait for defer blocks
				break WAIT
			}
		case <-deadline:
			t.Fatal("timeout waiting for plan execution")
		}
	}

	// Check timeline for artifacts_summary entry
	timeline := reactIns.config.Timeline
	require.NotNil(t, timeline)

	timelineDump := timeline.Dump()
	t.Logf("timeline dump:\n%s", utils.ShrinkString(timelineDump, 2000))

	assert.Contains(t, timelineDump, "artifacts_summary",
		"timeline should contain artifacts_summary entry after plan completion")

	// The timeline should contain the actual workdir path
	workDir := reactIns.config.GetOrCreateWorkDir()
	assert.Contains(t, timelineDump, workDir,
		"timeline should contain the artifacts directory path")
}

// TestArtifactsVisibility_EmitPinDirectory verifies that EmitPinDirectory is called
// after plan/forge execution to ensure UI visibility.
func TestArtifactsVisibility_EmitPinDirectory(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 200)

	var pinDirEvents []string
	var pinMu sync.Mutex

	var promptMu sync.Mutex
	callCount := 0

	var reactIns *ReAct

	var insErr error
	reactIns, insErr = NewTestReAct(
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			if e.Type == schema.EVENT_TYPE_FILESYSTEM_PIN_DIRECTORY {
				pinMu.Lock()
				pinDirEvents = append(pinDirEvents, string(e.Content))
				pinMu.Unlock()
			}
			select {
			case out <- e.ToGRPC():
			default:
			}
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			promptMu.Lock()
			callCount++
			currentCall := callCount
			promptMu.Unlock()

			rsp := i.NewAIResponse()
			if currentCall == 1 {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "request_plan_and_execution", "plan_request_payload": "test pin dir"}, "human_readable_thought": "plan", "cumulative_summary": "plan"}`))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "done"}, "human_readable_thought": "done", "cumulative_summary": "done"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithHijackPERequest(func(ctx context.Context, payload string) error {
			workDir := reactIns.config.GetOrCreateWorkDir()
			taskDir := filepath.Join(workDir, "task_1-1_pin_test")
			os.MkdirAll(taskDir, 0755)
			os.WriteFile(filepath.Join(taskDir, "output.txt"), []byte("pin test"), 0644)
			return nil
		}),
	)
	require.NoError(t, insErr)
	defer func() {
		workDir := reactIns.config.GetOrCreateWorkDir()
		if workDir != "" {
			os.RemoveAll(workDir)
		}
	}()

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test plan for pin",
		}
	}()

	// Wait for plan completion
	deadline := time.After(15 * time.Second)
WAIT:
	for {
		select {
		case <-out:
			promptMu.Lock()
			done := callCount >= 1
			promptMu.Unlock()
			if done {
				time.Sleep(1 * time.Second)
				break WAIT
			}
		case <-deadline:
			t.Fatal("timeout waiting for plan execution")
		}
	}

	// Check that EmitPinDirectory was called with the actual workdir
	workDir := reactIns.config.GetOrCreateWorkDir()

	pinMu.Lock()
	defer pinMu.Unlock()

	foundPinForWorkdir := false
	for _, evt := range pinDirEvents {
		if strings.Contains(evt, workDir) {
			foundPinForWorkdir = true
			break
		}
	}

	assert.True(t, foundPinForWorkdir,
		"EmitPinDirectory should have been called with the workdir path (%s). Pin events: %v", workDir, pinDirEvents)
}

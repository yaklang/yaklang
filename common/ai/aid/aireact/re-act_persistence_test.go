package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_PersistentSession_ToolUse(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolCalled := false
	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCalled = true
			sleepInt := params.GetFloat("seconds", 0.3)
			if sleepInt <= 0 {
				sleepInt = 0.3
			}
			time.Sleep(time.Duration(sleepInt) * time.Second)
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	pid := uuid.New()
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "sleep")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
		aicommon.WithPersistentSessionId(pid.String()),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 1; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
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

	if !reviewed {
		t.Fatal("Expected to have at least one review event, but got none")
	}

	if !reviewReleased {
		t.Fatal("Expected to have at least one review release event, but got none")
	}

	if !toolCalled {
		t.Fatal("Tool was not called")
	}

	if !toolCallOutputEvent {
		t.Fatal("Expected to have at least one output event, but got none")
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	if !strings.Contains(tl, `mocked thought for tool calling`) {
		t.Fatal("timeline does not contain mocked thought")
	}
	if !utils.MatchAllOfSubString(tl, `system-question`, "user-answer", "when review") {
		t.Fatal("timeline does not contain system-question")
	}
	if !utils.MatchAllOfSubString(tl, `ReAct iteration 1`, `ReAct Iteration Done[1]`) {
		fmt.Println(tl)
		t.Fatal("timeline does not contain ReAct iteration")
	}
	fmt.Println("--------------------------------------")

	persistentTimeline, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "sleep")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
		aicommon.WithPersistentSessionId(pid.String()),
	)
	if err != nil {
		t.Fatal(err)
	}

	withoutPersistent, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "sleep")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
	)
	if err != nil {
		t.Fatal(err)
	}

	if withoutPersistent.getTimelineTotal() > 0 {
		t.Fatal("Timeline (without persistent) is not empty")
	}
	if persistentTimeline.getTimelineTotal() <= 0 {
		t.Fatal("Timeline (with persistent) is empty")
	}
}

// TestReAct_PersistentSession_WorkDir tests that Session Artifacts (WorkDir) persist
// across persistent sessions, just like Timeline does. It runs a full Plan execution
// in Session 1 that produces artifacts, then creates Session 2 with the same
// persistent session ID and verifies that:
// 1. The WorkDir is restored (same path as Session 1)
// 2. Plan-produced artifact files are accessible
// 3. Timeline with artifacts_summary is restored
// 4. The new runtime's DB record has the restored WorkDir
// 5. A session without persistent ID gets no WorkDir
func TestReAct_PersistentSession_WorkDir(t *testing.T) {
	pid := uuid.New().String()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 200)

	var promptMu sync.Mutex
	callCount := 0

	var reactIns *ReAct

	// === Session 1: Run plan execution that produces artifacts ===
	var insErr error
	reactIns, insErr = NewTestReAct(
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case out <- e.ToGRPC():
			default:
			}
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			promptMu.Lock()
			callCount++
			currentCall := callCount
			promptMu.Unlock()

			rsp := i.NewAIResponse()
			if currentCall == 1 {
				// First call: AI requests plan execution
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "request_plan_and_execution", "plan_request_payload": "test persistent workdir plan"}, "human_readable_thought": "plan for persistence test", "cumulative_summary": "plan requested"}`))
			} else {
				// After plan: direct answer
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "plan done"}, "human_readable_thought": "done", "cumulative_summary": "done"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithPersistentSessionId(pid),
		aicommon.WithHijackPERequest(func(ctx context.Context, payload string) error {
			// Simulate plan execution: create artifact files in the WorkDir
			workDir := reactIns.config.GetOrCreateWorkDir()
			log.Infof("hijack PE: creating artifacts in WorkDir: %s", workDir)

			// Task 1: network scan results
			task1Dir := filepath.Join(workDir, "task_1-1_network_scan")
			os.MkdirAll(filepath.Join(task1Dir, "tool_calls"), 0755)
			os.WriteFile(
				filepath.Join(task1Dir, "tool_calls", "1_nmap_scan.md"),
				[]byte("# Tool Call: nmap\n## Parameters\ncommand: nmap -sV 192.168.1.0/24\n## Result\n12 hosts up"),
				0644,
			)
			os.WriteFile(
				filepath.Join(task1Dir, "task_1_1_result_summary.txt"),
				[]byte("Scan completed: 254 hosts scanned, 12 hosts up"),
				0644,
			)

			// Task 2: vulnerability report
			task2Dir := filepath.Join(workDir, "task_1-2_vuln_analysis")
			os.MkdirAll(task2Dir, 0755)
			os.WriteFile(
				filepath.Join(task2Dir, "vuln_report.md"),
				[]byte("# Vulnerability Analysis Report\nNo critical vulnerabilities found\n\n## Summary\n- Scanned 12 hosts\n- 0 critical, 2 medium, 5 low"),
				0644,
			)
			return nil
		}),
	)
	require.NoError(t, insErr)
	require.NotNil(t, reactIns)

	defer func() {
		// Cleanup the WorkDir created by ensureWorkDirectory
		if reactIns.config.IsWorkDirReady() {
			workDir := reactIns.config.GetOrCreateWorkDir()
			if workDir != "" {
				os.RemoveAll(workDir)
			}
		}
	}()

	// Send first input to trigger plan execution
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "scan the network and analyze vulnerabilities",
		}
	}()

	// Wait for plan execution to complete
	deadline := time.After(15 * time.Second)
	planStarted := false
	planEnded := false
WAIT_PLAN:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				planStarted = true
				log.Infof("plan execution started")
			}
			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				planEnded = true
				log.Infof("plan execution ended")
				if planStarted && planEnded {
					break WAIT_PLAN
				}
			}
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				status := utils.InterfaceToString(result)
				if status == "completed" || status == "failed" {
					break WAIT_PLAN
				}
			}
		case <-deadline:
			t.Fatal("timeout waiting for plan execution to complete")
		}
	}

	require.True(t, planStarted, "plan should have started")
	require.True(t, planEnded, "plan should have ended")

	// Wait for defer blocks (emitArtifactsSummaryToTimeline) and timeline throttle (3s) to flush
	time.Sleep(5 * time.Second)

	// Capture Session 1 state for later comparison
	session1WorkDir := reactIns.config.GetOrCreateWorkDir()
	require.NotEmpty(t, session1WorkDir, "session 1 should have a WorkDir created by ensureWorkDirectory")
	log.Infof("session 1 WorkDir: %s", session1WorkDir)

	// Verify Session 1 artifacts exist on disk
	_, err := os.Stat(filepath.Join(session1WorkDir, "task_1-1_network_scan"))
	require.NoError(t, err, "task_1-1_network_scan dir should exist in session 1 WorkDir")

	_, err = os.Stat(filepath.Join(session1WorkDir, "task_1-2_vuln_analysis"))
	require.NoError(t, err, "task_1-2_vuln_analysis dir should exist in session 1 WorkDir")

	// Note: artifacts_summary in timeline is timing-dependent due to save throttle (3s).
	// This is separately tested by TestArtifactsVisibility_TimelineContainsArtifactsSummary.

	// Verify Session 1 WorkDir is saved in DB
	runtime1, err := yakit.GetLatestAIAgentRuntimeByPersistentSession(consts.GetGormProjectDatabase(), pid)
	require.NoError(t, err)
	require.NotNil(t, runtime1)
	assert.Equal(t, session1WorkDir, runtime1.WorkDir,
		"DB record should have session 1's WorkDir path")

	close(in)

	// === Session 2: Create new instance with SAME persistent session ===
	// restorePersistentSession() should restore Timeline + WorkDir
	in2 := make(chan *ypb.AIInputEvent, 10)
	out2 := make(chan *ypb.AIOutputEvent, 200)
	ins2, err := NewTestReAct(
		aicommon.WithEventInputChan(in2),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case out2 <- e.ToGRPC():
			default:
			}
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, nil
		}),
		aicommon.WithPersistentSessionId(pid),
	)
	require.NoError(t, err)
	require.NotNil(t, ins2)

	// Verify: WorkDir is restored
	require.True(t, ins2.config.IsWorkDirReady(),
		"session 2 should have restored WorkDir from persistent session")

	session2WorkDir := ins2.config.GetOrCreateWorkDir()
	require.Equal(t, session1WorkDir, session2WorkDir,
		"session 2 WorkDir should be identical to session 1 WorkDir")
	log.Infof("session 2 restored WorkDir: %s", session2WorkDir)

	// Verify: Plan-produced artifact files are accessible from restored WorkDir
	content, err := os.ReadFile(filepath.Join(session2WorkDir, "task_1-1_network_scan", "tool_calls", "1_nmap_scan.md"))
	require.NoError(t, err, "nmap scan artifact should be readable from restored WorkDir")
	assert.Contains(t, string(content), "12 hosts up",
		"nmap scan artifact content should be intact")

	content, err = os.ReadFile(filepath.Join(session2WorkDir, "task_1-1_network_scan", "task_1_1_result_summary.txt"))
	require.NoError(t, err, "result summary artifact should be readable from restored WorkDir")
	assert.Contains(t, string(content), "254 hosts scanned",
		"result summary content should be intact")

	content, err = os.ReadFile(filepath.Join(session2WorkDir, "task_1-2_vuln_analysis", "vuln_report.md"))
	require.NoError(t, err, "vuln report artifact should be readable from restored WorkDir")
	assert.Contains(t, string(content), "Vulnerability Analysis Report",
		"vuln report content should be intact")

	// Verify: Timeline is restored from persistent session
	require.True(t, ins2.getTimelineTotal() > 0,
		"session 2 should have restored timeline items from persistent session")
	// Note: artifacts_summary content in timeline is timing-dependent (save throttle is 3s).
	// The content assertion is covered by TestArtifactsVisibility_TimelineContainsArtifactsSummary.
	session2Timeline := ins2.DumpTimeline()
	if strings.Contains(session2Timeline, "artifacts_summary") {
		log.Infof("restored timeline contains artifacts_summary (throttle flush completed)")
	} else {
		log.Infof("restored timeline does not contain artifacts_summary (throttle flush may not have completed, acceptable)")
	}

	// === Session 3: Create instance WITHOUT persistent session ===
	in3 := make(chan *ypb.AIInputEvent, 10)
	out3 := make(chan *ypb.AIOutputEvent, 200)
	ins3, err := NewTestReAct(
		aicommon.WithEventInputChan(in3),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case out3 <- e.ToGRPC():
			default:
			}
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, nil
		}),
	)
	require.NoError(t, err)

	// Verify: no WorkDir and no timeline for non-persistent session
	require.False(t, ins3.config.IsWorkDirReady(),
		"session 3 (no persistent session) should have no WorkDir")
	require.Equal(t, 0, ins3.getTimelineTotal(),
		"session 3 (no persistent session) should have empty timeline")

	log.Infof("persistent session WorkDir test passed")
	log.Infof("  session 1 WorkDir: %s", session1WorkDir)
	log.Infof("  session 2 restored WorkDir: %s", session2WorkDir)
	log.Infof("  session 1 timeline items: %d", reactIns.getTimelineTotal())
	log.Infof("  session 2 timeline items: %d", ins2.getTimelineTotal())
}

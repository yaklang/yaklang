package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type detachedApprovalTestServer struct {
	*Server
	options []aicommon.ConfigOption
}

func (s *detachedApprovalTestServer) StartAIReAct(stream ypb.Yak_StartAIReActServer) error {
	return s.startAIReActWithOptions(stream, false, s.options...)
}

// Exercise the frontend's protobuf transport, runtime input delivery and real
// executor together. A start acknowledgement alone does not mean a plan ran.
func TestStartAIReActDetachedApprovalExecutesAndKeepsStreamOpen(t *testing.T) {
	for _, interruptPlanning := range []bool{false, true} {
		t.Run(map[bool]string{false: "after_planning_completed", true: "cancel_then_approve"}[interruptPlanning], func(t *testing.T) {
			testStartAIReActDetachedApproval(t, interruptPlanning)
		})
	}
}

func testStartAIReActDetachedApproval(t *testing.T, interruptPlanning bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	base := newScheduleTestServer(t)
	var workers atomic.Int32
	var workerCalls sync.Map
	pattern := regexp.MustCompile(`\[([^\]]+)\]: ([a-z_]+); attempt=(\d+); observed=(true|false)`)
	srv := &detachedApprovalTestServer{Server: base, options: []aicommon.ConfigOption{
		aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithNoOpMemoryTriage(), aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true), aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisablePerception(true), aicommon.WithDisableAutoSkills(true),
		aicommon.WithGenerateReport(false), aicommon.WithDisableDynamicPlanning(true),
		aicommon.WithPeriodicVerificationInterval(0), aicommon.WithDisableIncreaseIteration(true),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
			if wire.ToolCallCallback == nil || wire.FinishReasonCallback == nil {
				return nil, fmt.Errorf("PLAN must use function calls: %s", req.GetCallerLabel())
			}
			respond := func(name string, args any) (*aicommon.AIResponse, error) {
				data, _ := json.Marshal(args)
				wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("rpc-%d", req.GetSeqId()), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(data)}}})
				wire.FinishReasonCallback("tool_calls", nil)
				rsp := c.NewAIResponse()
				rsp.Close()
				return rsp, nil
			}
			prompt := req.GetPrompt()
			if strings.Contains(req.GetCallerLabel(), "default") {
				return respond("request_plan_and_execution", map[string]any{"plan_request_payload": "Complete two dependent checks"})
			}
			if strings.Contains(prompt, "Execute the assigned frozen plan task.") {
				if _, loaded := workerCalls.LoadOrStore(req.GetTaskIndex(), true); !loaded {
					workers.Add(1)
					return respond("submit_task_result", map[string]any{"summary": "Check completed"})
				}
				return respond("finish", map[string]any{})
			}
			if strings.Contains(prompt, "Draft version: 0") {
				return respond("create_plan", map[string]any{"plan": map[string]any{"name": "RPC plan", "goal": "Two checks", "tasks": []any{map[string]any{"name": "First", "goal": "Check first", "identifier": "first", "depends_on": []string{}}, map[string]any{"name": "Second", "goal": "Verify first result", "identifier": "second", "depends_on": []string{"first"}}}}, "plan_document": "# Two dependent checks"})
			}
			if strings.Contains(prompt, "Detached submitted version:") {
				if interruptPlanning {
					<-c.GetContext().Done()
					return nil, c.GetContext().Err()
				}
				return respond("finish", map[string]any{})
			}
			if strings.Contains(prompt, "approved version: 0") {
				return respond("submit_plan", map[string]any{"plan_version": 1})
			}
			matches := pattern.FindAllStringSubmatch(prompt, -1)
			for _, m := range matches {
				if m[2] == "awaiting_review" {
					if m[4] == "false" {
						return respond("inspect_tasks", map[string]any{})
					}
					attempt, _ := strconv.Atoi(m[3])
					return respond("review_task", map[string]any{"task_id": m[1], "attempt_id": attempt, "decision": "accept", "reason": "Verified the result"})
				}
			}
			for _, m := range matches {
				if m[2] == "running" {
					return respond("wait_tasks", map[string]any{"timeout_seconds": 1})
				}
			}
			for _, m := range matches {
				if m[2] == "pending" {
					return respond("start_tasks", map[string]any{})
				}
			}
			return respond("finish", map[string]any{})
		}),
	}}
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()
	transport := grpc.NewServer()
	ypb.RegisterYakServer(transport, srv)
	defer transport.Stop()
	go transport.Serve(listener)
	conn, err := grpc.DialContext(ctx, "passthrough:///approval", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	stream, err := ypb.NewYakClient(conn).StartAIReAct(ctx)
	require.NoError(t, err)
	sessionID := "approval-grpc-" + uuid.NewString()
	require.NoError(t, stream.Send(&ypb.AIInputEvent{IsStart: true, Params: &ypb.AIStartParams{
		TimelineSessionID: sessionID, EnablePlan: true, EnableDetachedPlan: true,
		Source: "ai", ReviewPolicy: "yolo", DisableAISearchForge: true, DisableToolUse: true,
	}}))
	running, err := aireact.WaitRunningSession(sessionID, 5*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() {
		cancel()
		require.Eventually(t, func() bool { _, ok := aireact.GetRunningSession(sessionID); return !ok }, 3*time.Second, 10*time.Millisecond)
	})
	require.NoError(t, running.GetConfig().GetDB().AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
	require.NoError(t, stream.Send(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "Plan and execute two dependent checks"}))
	var executionID, planningID string
	var approval *ypb.AIInputEvent
	panels, starts, ends, pushes, pops := 0, 0, 0, 0, 0
	for {
		event, err := stream.Recv()
		require.NoError(t, err, "gRPC must remain open after approval")
		var data map[string]any
		_ = json.Unmarshal(event.Content, &data)
		switch schema.EventType(event.Type) {
		case schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE:
			panels++
			require.Equal(t, 1, panels)
			require.Zero(t, workers.Load())
			id := data["coordinator_id"].(string)
			// Match the installed Yakit review component: genExecTasks hides
			// the root, then each item mounts description/tools defaults. If
			// those change the list, even an untouched review is sent as an
			// edit, whose root_task is the first reconstructed top-level task.
			plans := data["plans"].(map[string]any)
			children := plans["root_task"].(map[string]any)["subtasks"].([]any)
			require.Len(t, children, 2)
			treeEdited := false
			for _, raw := range children {
				child := raw.(map[string]any)
				before, err := json.Marshal(child)
				require.NoError(t, err)
				child["description"] = ""
				if hints, _ := child["tools"].([]any); len(hints) == 0 {
					child["tools"] = []any{}
				}
				after, err := json.Marshal(child)
				require.NoError(t, err)
				treeEdited = treeEdited || string(before) != string(after)
			}
			payload := map[string]any{"coordinator_id": id}
			if treeEdited {
				first := children[0].(map[string]any)
				first["subtasks"] = []any{}
				plans["root_task"] = first
				payload["plans"] = plans
			}
			raw, err := json.Marshal(payload)
			require.NoError(t, err)
			approval = &ypb.AIInputEvent{IsSyncMessage: true, SyncType: "execute_detached_plan", SyncID: "one-click", SyncJsonInput: string(raw)}
			if interruptPlanning {
				require.NoError(t, stream.Send(&ypb.AIInputEvent{IsSyncMessage: true, SyncType: "react_cancel_task", SyncID: "cancel-planning", SyncJsonInput: fmt.Sprintf(`{"task_id":%q}`, planningID)}))
			}
		case schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE:
			t.Fatal("approved plan requested a second review")
		case schema.EVENT_TYPE_FAIL_REACT:
			t.Fatalf("stopping the planning task for approval must not emit a failure card: %s", event.Content)
		case schema.EVENT_TYPE_START_PLAN_AND_EXECUTION:
			starts++
		case schema.EVENT_TYPE_END_PLAN_AND_EXECUTION:
			ends++
		}
		if event.NodeId == "react_task_dequeue" {
			if planningID == "" {
				planningID, _ = data["react_task_id"].(string)
			} else {
				executionID, _ = data["react_task_id"].(string)
				require.Contains(t, data["react_task_input"], "执行已批准计划")
				require.NotContains(t, data["react_task_input"], "恢复执行")
			}
		}
		planningFinished := data["react_task_now_status"] == "completed" || (interruptPlanning && data["react_task_now_status"] == "skipped")
		if event.NodeId == "react_task_status_changed" && data["react_task_id"] == planningID && planningFinished && approval != nil {
			require.NoError(t, stream.Send(approval))
			approval = nil
		}
		if event.NodeId == "execute_detached_plan" {
			require.Nil(t, data["error"], string(event.Content))
		}
		if event.IsSync && event.SyncID == "cancel-planning" {
			require.Nil(t, data["error"], "the nested coordinator must not handle the parent's cancel request: %s", event.Content)
		}
		if event.NodeId == "system" {
			if data["type"] == "push_task" {
				pushes++
			}
			if data["type"] == "pop_task" {
				pops++
			}
		}
		if event.NodeId == "react_task_status_changed" && executionID != "" && data["react_task_id"] == executionID {
			status := data["react_task_now_status"]
			if status == "completed" || status == "aborted" {
				require.Equal(t, "completed", status)
				require.NoError(t, stream.Send(&ypb.AIInputEvent{IsSyncMessage: true, SyncType: "queue_info", SyncID: "after-completion"}))
			}
		}
		if event.IsSync && event.SyncID == "after-completion" {
			break
		}
	}
	require.Equal(t, 1, panels)
	require.Equal(t, 1, starts)
	require.Equal(t, 1, ends)
	require.Equal(t, 2, pushes)
	require.Equal(t, 2, pops)
	require.EqualValues(t, 2, workers.Load())
	// The synthetic execution entry must not be promoted as a new user query.
	config, ok := running.GetConfig().(*aicommon.Config)
	require.True(t, ok)
	for _, input := range config.GetUserInputHistory() {
		require.NotContains(t, input.UserInput, "执行已批准计划")
	}
}

package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newBatchTestReAct(t *testing.T, tool *aitool.Tool, callback aicommon.AICallbackType) *ReAct {
	t.Helper()
	if callback == nil {
		callback = func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call")
		}
	}
	react, err := NewTestReAct(
		aicommon.WithAICallback(callback),
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAgreeYOLO(),
		aicommon.WithDisableToolCallerIntervalReview(true),
	)
	require.NoError(t, err)
	return react
}

func batchTestResultParamInt(t *testing.T, result *aitool.ToolResult, key string) int64 {
	t.Helper()
	require.NotNil(t, result)
	switch params := result.Param.(type) {
	case aitool.InvokeParams:
		return params.GetInt(key)
	case map[string]any:
		return aitool.InvokeParams(params).GetInt(key)
	default:
		t.Fatalf("unexpected result param type %T", result.Param)
		return 0
	}
}

type batchArtifactManifestFixture struct {
	Tool       string `json:"tool"`
	CallToolID string `json:"call_tool_id"`
	Identifier string `json:"identifier"`
	Status     string `json:"status"`
	Success    bool   `json:"success"`
}

func batchArtifactDirs(t *testing.T, react *ReAct) []string {
	t.Helper()
	task := react.config.DefaultTask
	root := filepath.Join(
		react.config.Workdir,
		aicommon.BuildTaskDirName(task.GetIndex(), task.GetSemanticIdentifier()),
		"tool_calls",
	)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}
	sort.Strings(dirs)
	return dirs
}

func readBatchArtifactManifest(t *testing.T, dir string) batchArtifactManifestFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	require.NoError(t, err)
	var manifest batchArtifactManifestFixture
	require.NoError(t, json.Unmarshal(raw, &manifest))
	return manifest
}

func TestExecuteToolBatch_RejectsScalarRequest(t *testing.T) {
	tool, err := aitool.New(
		"batch_min_items_tool",
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			return "unexpected", nil
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)

	result, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name}},
	})
	require.Nil(t, result)
	require.ErrorContains(t, execErr, "at least 2")
}

func TestExecuteToolBatch_DirectBoundedConcurrencyAndOrderedCommit(t *testing.T) {
	var active int32
	var maxActive int32
	var completionMu sync.Mutex
	var completion []int

	tool, err := aitool.New(
		"batch_order_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithIntegerParam("delay_ms", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			current := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&maxActive)
				if current <= old || atomic.CompareAndSwapInt32(&maxActive, old, current) {
					break
				}
			}
			time.Sleep(time.Duration(params.GetInt("delay_ms")) * time.Millisecond)
			atomic.AddInt32(&active, -1)
			id := int(params.GetInt("id"))
			completionMu.Lock()
			completion = append(completion, id)
			completionMu.Unlock()
			return id, nil
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 2)

	request := &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "test call 0", Params: aitool.InvokeParams{"id": 0, "delay_ms": 180}},
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "test call 1", Params: aitool.InvokeParams{"id": 1, "delay_ms": 20}},
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "test call 2", Params: aitool.InvokeParams{"id": 2, "delay_ms": 20}},
	}}

	batchResult, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, request)
	require.NoError(t, execErr)
	require.Len(t, batchResult.Outcomes, 3)
	require.Equal(t, int32(2), atomic.LoadInt32(&maxActive), "invoke concurrency must use its own configured bound")
	require.NotEqual(t, []int{0, 1, 2}, completion, "test must observe out-of-order worker completion")

	for i, outcome := range batchResult.Outcomes {
		require.Equal(t, i, outcome.Index)
		require.Equal(t, aicommon.ToolCallStageDone, outcome.Stage)
		require.NotNil(t, outcome.Result)
		require.Equal(t, int64(i), batchTestResultParamInt(t, outcome.Result, "id"))
	}
	committed := react.config.DefaultTask.GetAllToolCallResults()
	require.Len(t, committed, 3)
	for i, item := range committed {
		require.Equal(t, int64(i), batchTestResultParamInt(t, item, "id"), "task commit order must follow the model array")
	}
	snapshot := react.config.BuildSessionSnapshotExecution(react.config.DefaultTask)
	require.Equal(t, 3, snapshot.ToolCallSuccess)
	require.Zero(t, snapshot.ToolCallFailed)
	require.Equal(t, 3, snapshot.ToolCallTotal, "each successful batch child must count as one tool call")
}

func TestExecuteToolBatch_ConcurrentEventsRemainIsolatedPerChild(t *testing.T) {
	var eventsMu sync.Mutex
	var events []*schema.AiOutputEvent
	tool, err := aitool.New(
		"batch_event_isolation_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithIntegerParam("delay_ms", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout, _ io.Writer) (any, error) {
			_, _ = fmt.Fprintf(stdout, "event-child-%d\n", params.GetInt("id"))
			time.Sleep(time.Duration(params.GetInt("delay_ms")) * time.Millisecond)
			return params.GetInt("id"), nil
		}),
	)
	require.NoError(t, err)
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call")
		}),
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAgreeYOLO(),
		aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil {
				return
			}
			copyEvent := *event
			copyEvent.Content = append([]byte(nil), event.Content...)
			copyEvent.ProcessesId = append([]string(nil), event.ProcessesId...)
			eventsMu.Lock()
			events = append(events, &copyEvent)
			eventsMu.Unlock()
		}),
	)
	require.NoError(t, err)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 3)
	request := &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "event child zero", Params: aitool.InvokeParams{"id": 0, "delay_ms": 120}},
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "event child one", Params: aitool.InvokeParams{"id": 1, "delay_ms": 40}},
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "event child two", Params: aitool.InvokeParams{"id": 2, "delay_ms": 5}},
	}}

	result, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, request)
	require.NoError(t, execErr)
	require.Len(t, result.Outcomes, 3)

	callIndex := make(map[string]int, len(request.Calls))
	for index := range request.Calls {
		callID := request.Calls[index].ExecutionCallID
		require.NotEmpty(t, callID)
		callIndex[callID] = index
	}
	type lifecycle struct {
		starts    int
		terminals int
		params    int
		results   int
	}
	byCall := make(map[string]*lifecycle, len(callIndex))
	for callID := range callIndex {
		byCall[callID] = new(lifecycle)
	}
	eventsMu.Lock()
	snapshot := append([]*schema.AiOutputEvent(nil), events...)
	eventsMu.Unlock()
	for _, event := range snapshot {
		state, related := byCall[event.CallToolID]
		if !related {
			continue
		}
		// Persistence derives the recovery block from CallToolID. Run the same
		// normalization here so this test locks the database/UI contract as well
		// as the in-memory emitter contract.
		event.NormalizeRecoveryBlock()
		require.Equal(t, event.CallToolID, event.RecoveryIndexID)
		require.Contains(t, event.ProcessesId, event.CallToolID)
		require.Equal(t, react.config.DefaultTask.GetId(), event.TaskId)
		var payload map[string]any
		if event.IsJson && len(event.Content) > 0 && json.Unmarshal(event.Content, &payload) == nil {
			if payloadCallID, ok := payload["call_tool_id"].(string); ok {
				require.Equal(t, event.CallToolID, payloadCallID, "an event payload must never point at a sibling call")
			}
		}
		switch event.Type {
		case schema.EVENT_TOOL_CALL_START:
			state.starts++
			require.True(t, event.IsRecoveryBlock, "only each child's start anchors its recovery block")
		case schema.EVENT_TOOL_CALL_DONE, schema.EVENT_TOOL_CALL_ERROR, schema.EVENT_TOOL_CALL_USER_CANCEL:
			state.terminals++
			require.False(t, event.IsRecoveryBlock)
		case schema.EVENT_TOOL_CALL_PARAM:
			state.params++
		case schema.EVENT_TOOL_CALL_RESULT:
			state.results++
		}
	}
	for callID, state := range byCall {
		require.Equal(t, 1, state.starts, "call %s must have exactly one start anchor", callID)
		require.Equal(t, 1, state.terminals, "call %s must have exactly one terminal event", callID)
		require.Equal(t, 1, state.params, "call %s must emit its own final params exactly once", callID)
		require.Equal(t, 1, state.results, "call %s must emit its own result exactly once", callID)
	}
}

func TestExecuteToolBatch_SameToolLiveOutputEventsStayBoundToTheirChild(t *testing.T) {
	type capturedEvent struct {
		sequence int
		event    *schema.AiOutputEvent
	}
	var (
		eventsMu sync.Mutex
		events   []capturedEvent
		started  sync.WaitGroup
	)
	started.Add(2)

	tool, err := aitool.New(
		"batch_live_same_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout, stderr io.Writer) (any, error) {
			id := int(params.GetInt("id"))
			// Do not produce either child's output until both callbacks really are
			// inside the same tool. This makes the fixture exercise concurrent,
			// same-name streams instead of two accidentally serialized calls.
			started.Done()
			started.Wait()
			_, _ = fmt.Fprintf(stdout, "stdout-child-%d-first\n", id)
			_, _ = fmt.Fprintf(stderr, "stderr-child-%d-first\n", id)
			if id == 0 {
				time.Sleep(35 * time.Millisecond)
			} else {
				time.Sleep(5 * time.Millisecond)
			}
			_, _ = fmt.Fprintf(stdout, "stdout-child-%d-last\n", id)
			_, _ = fmt.Fprintf(stderr, "stderr-child-%d-last\n", id)
			return fmt.Sprintf("result-child-%d", id), nil
		}),
	)
	require.NoError(t, err)
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call")
		}),
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAgreeYOLO(),
		aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil {
				return
			}
			copyEvent := *event
			copyEvent.Content = append([]byte(nil), event.Content...)
			copyEvent.StreamDelta = append([]byte(nil), event.StreamDelta...)
			copyEvent.ProcessesId = append([]string(nil), event.ProcessesId...)
			eventsMu.Lock()
			events = append(events, capturedEvent{sequence: len(events), event: &copyEvent})
			eventsMu.Unlock()
		}),
	)
	require.NoError(t, err)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 2)
	request := &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "live output child zero", Params: aitool.InvokeParams{"id": 0}},
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "live output child one", Params: aitool.InvokeParams{"id": 1}},
	}}

	result, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, request)
	require.NoError(t, execErr)
	require.Len(t, result.Outcomes, 2)

	callIndex := make(map[string]int, 2)
	for i := range request.Calls {
		callID := request.Calls[i].ExecutionCallID
		require.NotEmpty(t, callID)
		callIndex[callID] = i
	}

	type streamTrace struct {
		callID       string
		contentType  string
		nodeID       string
		startCount   int
		deltaCount   int
		finishCount  int
		lastDeltaSeq int
		finishSeq    int
		data         bytes.Buffer
	}
	traces := make(map[string]*streamTrace, 4)
	resultCount := make(map[string]int, 2)
	resultSeq := make(map[string]int, 2)
	resultContent := make(map[string]string, 2)
	eventsMu.Lock()
	snapshot := append([]capturedEvent(nil), events...)
	eventsMu.Unlock()
	for _, captured := range snapshot {
		event := captured.event
		childIndex, isChildEvent := callIndex[event.CallToolID]
		if !isChildEvent {
			continue
		}
		require.Contains(t, event.ProcessesId, event.CallToolID)
		require.Equal(t, event.CallToolID, event.RecoveryIndexID)
		for siblingID := range callIndex {
			if siblingID != event.CallToolID {
				require.NotContains(t, event.ProcessesId, siblingID, "child %d event must not inherit its sibling's process", childIndex)
			}
		}

		switch {
		case event.Type == schema.EVENT_TYPE_STREAM_START &&
			(event.ContentType == aicommon.TypeLogTool || event.ContentType == aicommon.TypeLogToolErrorOutput):
			writerID := event.GetStreamEventWriterId()
			require.NotEmpty(t, writerID)
			require.NotEqual(t, event.CallToolID, writerID, "a stream writer is unique inside a child call; it is not the call id itself")
			_, duplicate := traces[writerID]
			require.False(t, duplicate, "stdout and stderr, including across siblings, need different writer ids")
			traces[writerID] = &streamTrace{
				callID:       event.CallToolID,
				contentType:  event.ContentType,
				nodeID:       event.NodeId,
				startCount:   1,
				lastDeltaSeq: -1,
				finishSeq:    -1,
			}
		case event.Type == schema.EVENT_TYPE_STREAM &&
			(event.ContentType == aicommon.TypeLogTool || event.ContentType == aicommon.TypeLogToolErrorOutput):
			trace := traces[event.EventUUID]
			require.NotNil(t, trace, "every stream delta must reference its already-emitted stream_start writer id")
			require.Equal(t, trace.callID, event.CallToolID)
			require.Equal(t, trace.contentType, event.ContentType)
			require.Equal(t, trace.nodeID, event.NodeId)
			trace.deltaCount++
			trace.lastDeltaSeq = captured.sequence
			_, _ = trace.data.Write(event.StreamDelta)
		case event.Type == schema.EVENT_TYPE_STRUCTURED && event.NodeId == "stream-finished":
			writerID := event.GetStreamEventWriterId()
			trace := traces[writerID]
			if trace == nil {
				continue
			}
			require.Equal(t, trace.callID, event.CallToolID)
			trace.finishCount++
			trace.finishSeq = captured.sequence
		case event.Type == schema.EVENT_TOOL_CALL_RESULT:
			resultCount[event.CallToolID]++
			resultSeq[event.CallToolID] = captured.sequence
			resultContent[event.CallToolID] = string(event.Content)
		}
	}

	require.Len(t, traces, 4, "two concurrent children must each own a stdout and stderr writer")
	nodesByContentType := map[string]map[string]struct{}{
		aicommon.TypeLogTool:            {},
		aicommon.TypeLogToolErrorOutput: {},
	}
	streamsPerChild := make(map[string]map[string]*streamTrace, 2)
	for writerID, trace := range traces {
		require.Equal(t, 1, trace.startCount, "writer %s must start exactly once", writerID)
		require.Positive(t, trace.deltaCount, "writer %s must deliver live data", writerID)
		require.Equal(t, 1, trace.finishCount, "writer %s must finish exactly once", writerID)
		require.Greater(t, trace.finishSeq, trace.lastDeltaSeq, "stream-finished must follow that writer's final flush")
		nodesByContentType[trace.contentType][trace.nodeID] = struct{}{}
		if streamsPerChild[trace.callID] == nil {
			streamsPerChild[trace.callID] = make(map[string]*streamTrace, 2)
		}
		_, duplicate := streamsPerChild[trace.callID][trace.contentType]
		require.False(t, duplicate, "one child must not receive two %s writers", trace.contentType)
		streamsPerChild[trace.callID][trace.contentType] = trace
	}
	// Both children intentionally call the same tool, so their node ids are the
	// same. Correct attribution therefore has to come from call/process and
	// writer identity rather than the human-readable node name.
	require.Len(t, nodesByContentType[aicommon.TypeLogTool], 1)
	require.Len(t, nodesByContentType[aicommon.TypeLogToolErrorOutput], 1)

	for callID, childIndex := range callIndex {
		streams := streamsPerChild[callID]
		require.Len(t, streams, 2)
		stdoutTrace := streams[aicommon.TypeLogTool]
		stderrTrace := streams[aicommon.TypeLogToolErrorOutput]
		require.NotNil(t, stdoutTrace)
		require.NotNil(t, stderrTrace)
		require.Equal(t, fmt.Sprintf("stdout-child-%d-first\nstdout-child-%d-last\n", childIndex, childIndex), stdoutTrace.data.String())
		require.Equal(t, fmt.Sprintf("stderr-child-%d-first\nstderr-child-%d-last\n", childIndex, childIndex), stderrTrace.data.String())
		for siblingIndex := range request.Calls {
			if siblingIndex == childIndex {
				continue
			}
			require.NotContains(t, stdoutTrace.data.String(), fmt.Sprintf("child-%d", siblingIndex))
			require.NotContains(t, stderrTrace.data.String(), fmt.Sprintf("child-%d", siblingIndex))
		}
		require.Equal(t, 1, resultCount[callID])
		require.Contains(t, resultContent[callID], fmt.Sprintf("result-child-%d", childIndex))
		require.Greater(t, resultSeq[callID], stdoutTrace.finishSeq, "tool result must follow the child's stdout final flush")
		require.Greater(t, resultSeq[callID], stderrTrace.finishSeq, "tool result must follow the child's stderr final flush")
	}
}

func TestExecuteToolBatch_ArtifactsAreIsolatedAndOrderedDespiteOutOfOrderCompletion(t *testing.T) {
	var completionMu sync.Mutex
	completion := make([]int, 0, 3)
	tool, err := aitool.New(
		"batch_artifact_isolation_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithIntegerParam("delay_ms", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout, stderr io.Writer) (any, error) {
			id := int(params.GetInt("id"))
			_, _ = fmt.Fprintf(stdout, "stdout-child-%d\n", id)
			_, _ = fmt.Fprintf(stderr, "stderr-child-%d\n", id)
			time.Sleep(time.Duration(params.GetInt("delay_ms")) * time.Millisecond)
			completionMu.Lock()
			completion = append(completion, id)
			completionMu.Unlock()
			return fmt.Sprintf("result-child-%d", id), nil
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 3)
	identifiers := []string{"model_zero", "model_one", "model_two"}
	request := &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Identifier: identifiers[0], Reason: "artifact 0", Params: aitool.InvokeParams{"id": 0, "delay_ms": 180}},
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Identifier: identifiers[1], Reason: "artifact 1", Params: aitool.InvokeParams{"id": 1, "delay_ms": 70}},
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Identifier: identifiers[2], Reason: "artifact 2", Params: aitool.InvokeParams{"id": 2, "delay_ms": 10}},
	}}

	result, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, request)
	require.NoError(t, execErr)
	require.Len(t, result.Outcomes, 3)
	completionMu.Lock()
	require.NotEqual(t, []int{0, 1, 2}, completion, "fixture must complete out of model order")
	completionMu.Unlock()

	dirs := batchArtifactDirs(t, react)
	require.Len(t, dirs, 3)
	resultIDs := make(map[int64]struct{}, 3)
	for i, dir := range dirs {
		base := filepath.Base(dir)
		require.True(t, strings.HasPrefix(base, fmt.Sprintf("%d_", i+1)), "artifact ordinal must follow model index: %s", base)
		require.Contains(t, base, identifiers[i])
		manifest := readBatchArtifactManifest(t, dir)
		require.Equal(t, identifiers[i], manifest.Identifier)
		require.Equal(t, request.Calls[i].ExecutionCallID, manifest.CallToolID)
		require.Equal(t, "success", manifest.Status)
		require.True(t, manifest.Success)

		stdout, readErr := os.ReadFile(filepath.Join(dir, "stdout.txt"))
		require.NoError(t, readErr)
		stderr, readErr := os.ReadFile(filepath.Join(dir, "stderr.txt"))
		require.NoError(t, readErr)
		combined, readErr := os.ReadFile(filepath.Join(dir, "combined_output.txt"))
		require.NoError(t, readErr)
		resultBytes, readErr := os.ReadFile(filepath.Join(dir, "result.txt"))
		require.NoError(t, readErr)
		require.Equal(t, fmt.Sprintf("stdout-child-%d\n", i), string(stdout))
		require.Equal(t, fmt.Sprintf("stderr-child-%d\n", i), string(stderr))
		require.Contains(t, string(combined), fmt.Sprintf("stdout-child-%d", i))
		require.Contains(t, string(combined), fmt.Sprintf("stderr-child-%d", i))
		require.Equal(t, fmt.Sprintf("result-child-%d", i), string(resultBytes))
		for other := 0; other < 3; other++ {
			if other == i {
				continue
			}
			require.NotContains(t, string(combined), fmt.Sprintf("child-%d", other))
			require.NotContains(t, string(resultBytes), fmt.Sprintf("child-%d", other))
		}

		outcome := result.Outcomes[i]
		require.Equal(t, i, outcome.Index)
		require.NotNil(t, outcome.Result)
		_, duplicate := resultIDs[outcome.Result.ID]
		require.False(t, duplicate, "every child must have a unique Timeline result ID")
		resultIDs[outcome.Result.ID] = struct{}{}
	}
	require.Equal(t, []int64{
		result.Outcomes[0].Result.ID,
		result.Outcomes[1].Result.ID,
		result.Outcomes[2].Result.ID,
	}, react.config.Timeline.GetTimelineItemIDs(), "Timeline must commit in model-index order")
}

func TestExecuteToolBatch_DirectAdmissionFailureStartsNothing(t *testing.T) {
	var invoked int32
	tool, err := aitool.New(
		"batch_validate_tool",
		aitool.WithStringParam("required_value", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "unexpected", nil
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)

	batchResult, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "valid test call", Params: aitool.InvokeParams{"required_value": "valid"}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "invalid test call", Params: aitool.InvokeParams{}},
		},
	})
	require.NoError(t, execErr)
	require.Equal(t, int32(0), atomic.LoadInt32(&invoked))
	require.Equal(t, aicommon.ToolCallStageCancelled, batchResult.Outcomes[0].Stage)
	require.Equal(t, aicommon.ToolCallStageValidationFailed, batchResult.Outcomes[1].Stage)
}

func TestExecuteToolBatch_FreshRequestReplaysStableCheckpointIdentity(t *testing.T) {
	var invoked int32
	tool, err := aitool.New(
		"batch_checkpoint_replay_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "executed", nil
		}),
	)
	require.NoError(t, err)
	runtimeID := "batch-replay-" + ksuid.New().String()
	const sequenceStart int64 = 9100
	newRuntime := func() *ReAct {
		react, runtimeErr := NewTestReAct(
			aicommon.WithID(runtimeID),
			aicommon.WithSequence(sequenceStart),
			aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				return nil, fmt.Errorf("unexpected AI call")
			}),
			aicommon.WithTools(tool),
			aicommon.WithWorkdir(t.TempDir()),
			aicommon.WithAgreeYOLO(),
			aicommon.WithDisableToolCallerIntervalReview(true),
		)
		require.NoError(t, runtimeErr)
		return react
	}
	newRequest := func() *aicommon.ToolBatchRequest {
		return &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "replay call 0", Params: aitool.InvokeParams{"id": 0}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "replay call 1", Params: aitool.InvokeParams{"id": 1}},
		}}
	}

	firstRuntime := newRuntime()
	firstRequest := newRequest()
	firstResult, firstErr := firstRuntime.ExecuteToolBatch(context.Background(), firstRuntime.config.DefaultTask, firstRequest)
	require.NoError(t, firstErr)
	require.Equal(t, int32(2), atomic.LoadInt32(&invoked))
	require.Len(t, firstResult.Outcomes, 2)
	firstBatchID := firstRequest.BatchID
	firstCallIDs := []string{firstRequest.Calls[0].ExecutionCallID, firstRequest.Calls[1].ExecutionCallID}

	// Simulate recovery by constructing a new runtime and a newly parsed request:
	// neither carries IDs from the first in-memory objects.
	secondRuntime := newRuntime()
	secondRequest := newRequest()
	secondResult, secondErr := secondRuntime.ExecuteToolBatch(context.Background(), secondRuntime.config.DefaultTask, secondRequest)
	require.NoError(t, secondErr)
	require.Equal(t, int32(2), atomic.LoadInt32(&invoked), "finished checkpoints must replay without invoking plugins again")
	require.Equal(t, firstBatchID, secondRequest.BatchID)
	require.Equal(t, firstCallIDs, []string{secondRequest.Calls[0].ExecutionCallID, secondRequest.Calls[1].ExecutionCallID})
	for _, outcome := range secondResult.Outcomes {
		require.NotNil(t, outcome.Result)
		require.True(t, outcome.Result.Success, "replayed result: %+v; outcome error: %v", outcome.Result, outcome.Err)
		require.Equal(t, aicommon.ToolCallStageDone, outcome.Stage)
	}
}

func TestExecuteToolBatch_CheckpointReplayCompactArtifactDoesNotCreateDuplicateBundle(t *testing.T) {
	var invoked int32
	tool, err := aitool.New(
		"batch_compact_replay_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			value := fmt.Sprintf("same-child-%d\n", params.GetInt("id"))
			_, _ = io.WriteString(stdout, value)
			return value, nil
		}),
	)
	require.NoError(t, err)
	runtimeID := "batch-compact-replay-" + ksuid.New().String()
	workdir := t.TempDir()
	const sequenceStart int64 = 9300
	newRuntime := func() *ReAct {
		react, runtimeErr := NewTestReAct(
			aicommon.WithID(runtimeID),
			aicommon.WithSequence(sequenceStart),
			aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				return nil, fmt.Errorf("unexpected AI call")
			}),
			aicommon.WithTools(tool),
			aicommon.WithWorkdir(workdir),
			aicommon.WithAgreeYOLO(),
			aicommon.WithDisableToolCallerIntervalReview(true),
		)
		require.NoError(t, runtimeErr)
		return react
	}
	newRequest := func() *aicommon.ToolBatchRequest {
		return &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Identifier: "compact_alpha", Reason: "compact replay 0", Params: aitool.InvokeParams{"id": 0}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Identifier: "compact_beta", Reason: "compact replay 1", Params: aitool.InvokeParams{"id": 1}},
		}}
	}

	firstRuntime := newRuntime()
	firstResult, firstErr := firstRuntime.ExecuteToolBatch(context.Background(), firstRuntime.config.DefaultTask, newRequest())
	require.NoError(t, firstErr)
	require.Equal(t, int32(2), atomic.LoadInt32(&invoked))
	require.Len(t, firstResult.Outcomes, 2)
	firstData := []string{
		firstResult.Outcomes[0].Result.Data.(string),
		firstResult.Outcomes[1].Result.Data.(string),
	}
	for _, data := range firstData {
		require.NotContains(t, data, "\n\nRESULT:\n", "combined==result uses the legal compact current format")
		require.Contains(t, data, "\n\nARTIFACT:\n")
	}
	require.Len(t, batchArtifactDirs(t, firstRuntime), 2)

	secondRuntime := newRuntime()
	secondResult, secondErr := secondRuntime.ExecuteToolBatch(context.Background(), secondRuntime.config.DefaultTask, newRequest())
	require.NoError(t, secondErr)
	require.Equal(t, int32(2), atomic.LoadInt32(&invoked), "checkpoint replay must not invoke plugins")
	require.Len(t, secondResult.Outcomes, 2)
	for i, outcome := range secondResult.Outcomes {
		require.Equal(t, firstData[i], outcome.Result.Data, "canonical compact Data must remain byte-stable")
	}
	dirs := batchArtifactDirs(t, secondRuntime)
	require.Len(t, dirs, 2, "speculative replay bundles must be removed instead of surviving as _2")
	for _, dir := range dirs {
		require.False(t, strings.HasSuffix(filepath.Base(dir), "_2"), "duplicate replay artifact survived: %s", dir)
	}
}

func TestExecuteToolBatch_ReviewCardsFollowModelArrayOrder(t *testing.T) {
	var invoked int32
	var reviewMu sync.Mutex
	var reviewedIDs []int
	input := make(chan *ypb.AIInputEvent, 4)
	tool, err := aitool.New(
		"batch_ordered_review_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "ok", nil
		}),
	)
	require.NoError(t, err)
	type reviewMaterial struct {
		ID     string `json:"id"`
		Params struct {
			ID int `json:"id"`
		} `json:"params"`
	}
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call")
		}),
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEventInputChan(input),
		aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil || event.Type != schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				return
			}
			var material reviewMaterial
			if json.Unmarshal(event.Content, &material) != nil {
				return
			}
			reviewMu.Lock()
			reviewedIDs = append(reviewedIDs, material.Params.ID)
			reviewMu.Unlock()
			input <- &ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        material.ID,
				InteractiveJSONInput: `{"suggestion":"continue"}`,
			}
		}),
	)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	batchResult, execErr := react.ExecuteToolBatch(ctx, react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "review call 0", Params: aitool.InvokeParams{"id": 0}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "review call 1", Params: aitool.InvokeParams{"id": 1}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "review call 2", Params: aitool.InvokeParams{"id": 2}},
		},
	})
	require.NoError(t, execErr)
	require.Len(t, batchResult.Outcomes, 3)
	require.Equal(t, int32(3), atomic.LoadInt32(&invoked))
	reviewMu.Lock()
	require.Equal(t, []int{0, 1, 2}, reviewedIDs)
	reviewMu.Unlock()
}

func TestExecuteToolBatch_ReviewCheckpointIdentityMismatchIsRejected(t *testing.T) {
	var invoked int32
	tool, err := aitool.New(
		"batch_review_checkpoint_tool",
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "must not invoke", nil
		}),
	)
	require.NoError(t, err)
	runtimeID := "batch-review-checkpoint-" + ksuid.New().String()
	const sequenceStart int64 = 9600
	react, err := NewTestReAct(
		aicommon.WithID(runtimeID),
		aicommon.WithSequence(sequenceStart),
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call")
		}),
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableToolCallerIntervalReview(true),
	)
	require.NoError(t, err)
	// Allocation layout is batch seed, then param/review/tool/watcher/result for
	// each child. Seed a conflicting review at the reserved child-0 review seq.
	reviewCheckpoint := react.config.CreateReviewCheckpoint(sequenceStart + 2)
	require.NoError(t, react.config.SubmitCheckpointRequest(reviewCheckpoint, map[string]any{
		"batch_id":     "wrong-batch",
		"call_index":   0,
		"call_tool_id": "wrong-call",
		"tool":         tool.Name,
		"params":       aitool.InvokeParams{},
	}))
	reviewCheckpoint2 := react.config.CreateReviewCheckpoint(sequenceStart + 7)
	require.NoError(t, react.config.SubmitCheckpointRequest(reviewCheckpoint2, map[string]any{
		"batch_id":     "wrong-batch",
		"call_index":   1,
		"call_tool_id": "wrong-call-2",
		"tool":         tool.Name,
		"params":       aitool.InvokeParams{},
	}))

	batchResult, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "identity mismatch 0", Params: aitool.InvokeParams{}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "identity mismatch 1", Params: aitool.InvokeParams{}},
		},
	})
	require.NoError(t, execErr)
	require.Equal(t, int32(0), atomic.LoadInt32(&invoked))
	require.Equal(t, aicommon.ToolCallStagePrepareFailed, batchResult.Outcomes[0].Stage)
	require.ErrorContains(t, batchResult.Outcomes[0].Err, "review checkpoint identity mismatch")
}

func TestExecuteToolBatch_RequireBoundsParamGenerationSeparately(t *testing.T) {
	var activeAI int32
	var maxActiveAI int32
	var invoked int32
	twoAIActive := make(chan struct{})
	var releaseAI sync.Once
	type childContextMarker struct{}
	tool, err := aitool.New(
		"batch_require_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "ok", nil
		}),
	)
	require.NoError(t, err)

	var missingChildContext int32
	callback := func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		if req.GetContext() == nil || req.GetContext().Value(childContextMarker{}) != "batch-child" {
			atomic.StoreInt32(&missingChildContext, 1)
		}
		current := atomic.AddInt32(&activeAI, 1)
		for {
			old := atomic.LoadInt32(&maxActiveAI)
			if current <= old || atomic.CompareAndSwapInt32(&maxActiveAI, old, current) {
				break
			}
		}
		if current >= 2 {
			releaseAI.Do(func() { close(twoAIActive) })
		}
		select {
		case <-twoAIActive:
		case <-time.After(3 * time.Second):
		}
		atomic.AddInt32(&activeAI, -1)
		response := config.NewAIResponse()
		response.EmitOutputStream(bytes.NewBufferString(`{"@action":"call-tool","identifier":"batch","params":{"id":1}}`))
		response.Close()
		return response, nil
	}
	react := newBatchTestReAct(t, tool, callback)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchParamConcurrency, 2)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 3)

	batchCtx := context.WithValue(context.Background(), childContextMarker{}, "batch-child")
	batchResult, execErr := react.ExecuteToolBatch(batchCtx, react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "first"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "second"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "third"},
		},
	})
	require.NoError(t, execErr)
	require.Len(t, batchResult.Outcomes, 3)
	require.Equal(t, int32(2), atomic.LoadInt32(&maxActiveAI))
	require.Equal(t, int32(0), atomic.LoadInt32(&missingChildContext), "parameter AI requests must carry the child context")
	require.Equal(t, int32(3), atomic.LoadInt32(&invoked))
	for _, outcome := range batchResult.Outcomes {
		require.Equal(t, aicommon.ToolCallStageDone, outcome.Stage)
	}
}

func TestExecuteToolBatch_RequireParamFailureIsAllSettled(t *testing.T) {
	var aiCalls int32
	var invoked int32
	tool, err := aitool.New(
		"batch_require_all_settled_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "ok", nil
		}),
	)
	require.NoError(t, err)
	callback := func(config aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		if atomic.AddInt32(&aiCalls, 1) == 1 {
			return nil, fmt.Errorf("synthetic parameter generation failure")
		}
		response := config.NewAIResponse()
		response.EmitOutputStream(bytes.NewBufferString(`{"@action":"call-tool","params":{"id":1}}`))
		response.Close()
		return response, nil
	}
	react := newBatchTestReAct(t, tool, callback)
	react.config.AiAutoRetry = 1
	react.config.AiTransactionAutoRetry = 1
	react.config.SetConfig(aicommon.ConfigKeyToolBatchParamConcurrency, 3)

	batchResult, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "first"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "second"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "third"},
		},
	})
	require.NoError(t, execErr)
	require.Len(t, batchResult.Outcomes, 3)
	require.Equal(t, int32(2), atomic.LoadInt32(&invoked), "one prepare failure must not cancel successful siblings")
	var preparedFailed, done int
	for _, outcome := range batchResult.Outcomes {
		switch outcome.Stage {
		case aicommon.ToolCallStagePrepareFailed:
			preparedFailed++
		case aicommon.ToolCallStageDone:
			done++
		}
	}
	require.Equal(t, 1, preparedFailed)
	require.Equal(t, 2, done)
}

func TestExecuteToolBatch_RequireParamCancellationDoesNotRetry(t *testing.T) {
	var aiCalls int32
	started := make(chan struct{})
	tool, err := aitool.New(
		"batch_require_cancel_params_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			return "must not invoke", nil
		}),
	)
	require.NoError(t, err)
	callback := func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		if atomic.AddInt32(&aiCalls, 1) == 1 {
			close(started)
		}
		requestCtx := req.GetContext()
		if requestCtx == nil {
			return nil, fmt.Errorf("missing request context")
		}
		<-requestCtx.Done()
		return nil, requestCtx.Err()
	}
	react := newBatchTestReAct(t, tool, callback)
	react.config.AiAutoRetry = 5
	react.config.AiTransactionAutoRetry = 5
	react.config.SetConfig(aicommon.ConfigKeyToolBatchParamConcurrency, 1)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var batchResult *aicommon.ToolBatchResult
	var execErr error
	go func() {
		batchResult, execErr = react.ExecuteToolBatch(ctx, react.config.DefaultTask, &aicommon.ToolBatchRequest{
			Calls: []aicommon.ToolBatchCall{
				{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "cancel while generating params 0"},
				{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "cancel while generating params 1"},
			},
		})
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("parameter AI callback did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("parameter transaction did not stop on child cancellation")
	}
	require.ErrorIs(t, execErr, context.Canceled)
	require.Equal(t, int32(1), atomic.LoadInt32(&aiCalls), "cancellation must not enter gateway or transaction retries")
	require.NotNil(t, batchResult)
	require.Equal(t, aicommon.ToolCallStageCancelled, batchResult.Outcomes[0].Stage)
}

func TestExecuteToolBatch_DirectAnswerCancelsWholeBatchBeforeAnyInvoke(t *testing.T) {
	var invoked int32
	var reviewCount int32
	input := make(chan *ypb.AIInputEvent, 4)
	tool, err := aitool.New(
		"batch_direct_answer_tool",
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "must not run", nil
		}),
	)
	require.NoError(t, err)
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call")
		}),
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEventInputChan(input),
		aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil || event.Type != schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				return
			}
			atomic.AddInt32(&reviewCount, 1)
			input <- &ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        event.GetContentJSONPath("$.id"),
				InteractiveJSONInput: `{"suggestion":"direct_answer"}`,
			}
		}),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	batchResult, execErr := react.ExecuteToolBatch(ctx, react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "direct answer call 0", Params: aitool.InvokeParams{}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "direct answer call 1", Params: aitool.InvokeParams{}},
		},
	})
	require.NoError(t, execErr)
	require.True(t, batchResult.DirectlyAnswer)
	require.GreaterOrEqual(t, atomic.LoadInt32(&reviewCount), int32(1))
	require.Equal(t, int32(0), atomic.LoadInt32(&invoked), "the final barrier must keep every plugin callback at zero")
	for _, outcome := range batchResult.Outcomes {
		require.Equal(t, aicommon.ToolCallStageCancelled, outcome.Stage)
	}
	require.Empty(t, batchArtifactDirs(t, react), "direct-answer before invoke must not create artifact bundles")
}

func TestExecuteToolBatch_OrdinaryFailureRetainsFailedArtifact(t *testing.T) {
	tool, err := aitool.New(
		"batch_failed_artifact_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout, stderr io.Writer) (any, error) {
			id := params.GetInt("id")
			_, _ = fmt.Fprintf(stdout, "stdout-before-failure-%d\n", id)
			_, _ = fmt.Fprintf(stderr, "stderr-before-failure-%d\n", id)
			if id == 0 {
				return nil, fmt.Errorf("synthetic ordinary failure %d", id)
			}
			return fmt.Sprintf("success-result-%d", id), nil
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)
	result, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Identifier: "failed_child", Reason: "fail normally", Params: aitool.InvokeParams{"id": 0}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Identifier: "successful_child", Reason: "succeed normally", Params: aitool.InvokeParams{"id": 1}},
		},
	})
	require.NoError(t, execErr, "batch is all-settled even when one child fails")
	require.Equal(t, aicommon.ToolCallStageInvokeFailed, result.Outcomes[0].Stage)
	require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[1].Stage)
	snapshot := react.config.BuildSessionSnapshotExecution(react.config.DefaultTask)
	require.Equal(t, 1, snapshot.ToolCallSuccess)
	require.Equal(t, 1, snapshot.ToolCallFailed)
	require.Equal(t, 2, snapshot.ToolCallTotal)
	dirs := batchArtifactDirs(t, react)
	require.Len(t, dirs, 2)
	failedDir := dirs[0]
	require.Contains(t, filepath.Base(failedDir), "failed_child")
	manifest := readBatchArtifactManifest(t, failedDir)
	require.Equal(t, "failed", manifest.Status)
	require.False(t, manifest.Success)
	require.Contains(t, manifest.Identifier, "failed_child")
	combined, err := os.ReadFile(filepath.Join(failedDir, "combined_output.txt"))
	require.NoError(t, err)
	require.Contains(t, string(combined), "stdout-before-failure-0")
	require.Contains(t, string(combined), "stderr-before-failure-0")
	require.Contains(t, string(combined), "synthetic ordinary failure 0")
	require.FileExists(t, filepath.Join(failedDir, "report.md"))
}

func TestExecuteToolBatch_CancellationReachesRunningChildren(t *testing.T) {
	started := make(chan struct{}, 2)
	allowLateWrites := make(chan struct{})
	lateWritesDone := make(chan struct{}, 2)
	tool, err := aitool.New(
		"batch_cancel_tool",
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithNoRuntimeCallback(func(ctx context.Context, _ aitool.InvokeParams, stdout, stderr io.Writer) (any, error) {
			started <- struct{}{}
			<-ctx.Done()
			// Deliberately outlive ToolCaller cancellation. The caller must be able
			// to return, close and remove its unfinished bundle while this plugin
			// goroutine still holds the stream writers.
			<-allowLateWrites
			_, _ = io.WriteString(stdout, "late stdout after cancellation\n")
			_, _ = io.WriteString(stderr, "late stderr after cancellation\n")
			lateWritesDone <- struct{}{}
			return nil, ctx.Err()
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 2)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var batchResult *aicommon.ToolBatchResult
	var execErr error
	go func() {
		batchResult, execErr = react.ExecuteToolBatch(ctx, react.config.DefaultTask, &aicommon.ToolBatchRequest{
			Calls: []aicommon.ToolBatchCall{
				{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "cancel call 0", Params: aitool.InvokeParams{}},
				{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "cancel call 1", Params: aitool.InvokeParams{}},
			},
		})
		close(done)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("tool child did not start")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("batch did not stop after cancellation")
	}
	require.ErrorIs(t, execErr, context.Canceled)
	require.NotNil(t, batchResult)
	for _, outcome := range batchResult.Outcomes {
		require.Equal(t, aicommon.ToolCallStageCancelled, outcome.Stage)
		if outcome.Result != nil {
			require.False(t, outcome.Result.Success)
		}
	}
	require.Empty(t, react.config.DefaultTask.GetAllToolCallResults(), "cancelled children must not be committed to task state")
	var finishedToolCheckpoints int
	require.NoError(t, react.config.GetDB().Model(&schema.AiCheckpoint{}).
		Where("coordinator_uuid = ? AND type = ? AND finished = ?", react.config.GetRuntimeId(), schema.AiCheckpointType_ToolCall, true).
		Count(&finishedToolCheckpoints).Error)
	require.Zero(t, finishedToolCheckpoints, "cancelled plugin executions must leave replayable unfinished checkpoints")
	require.Empty(t, batchArtifactDirs(t, react), "cancelled running children must roll back unfinished artifact bundles")
	close(allowLateWrites)
	for i := 0; i < 2; i++ {
		select {
		case <-lateWritesDone:
		case <-time.After(2 * time.Second):
			t.Fatal("non-cooperative callback did not finish its late write")
		}
	}
	require.Empty(t, batchArtifactDirs(t, react), "late plugin writes must not recreate cancelled artifact bundles")
	snapshot := react.config.BuildSessionSnapshotExecution(react.config.DefaultTask)
	require.Zero(t, snapshot.ToolCallTotal, "cancelled children without committed results must not count as completed tool calls")
}

func TestToolBatchBarrier_DirectAnswerAbortsReadySibling(t *testing.T) {
	barrier := newToolBatchBarrier(2)
	invokeAcquired := int32(0)
	invokeGate := func(context.Context) (func(), error) {
		atomic.AddInt32(&invokeAcquired, 1)
		return func() {}, nil
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := barrier.wait(context.Background(), 0, invokeGate)
		errCh <- err
	}()
	barrier.arrive(1, true)
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, errToolBatchDirectAnswer)
	case <-time.After(time.Second):
		t.Fatal("ready sibling remained blocked")
	}
	require.Equal(t, int32(0), atomic.LoadInt32(&invokeAcquired))
}

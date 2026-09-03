package aireact

import (
	"bytes"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestDisableMemoryTriage_InstallsNoOp verifies that when
// WithDisableMemoryTriage(true) is passed, the ReAct instance uses a
// no-op MemoryTriage instead of creating a real AIMemory (which would
// require embedding/DB/AI calls).
func TestDisableMemoryTriage_InstallsNoOp(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	ins, err := NewReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedFreeInputOutput(i, "disable-mem-flag")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisablePerception(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableDynamicPlanning(true),
		aicommon.WithPeriodicVerificationInterval(0),
		aicommon.WithDisableIncreaseIteration(true),
		// The key option under test:
		aicommon.WithDisableMemoryTriage(true),
	)
	require.NoError(t, err)

	// The memory triage must be a no-op, not a real AIMemory.
	require.NotNil(t, ins.memoryTriage)
	require.Equal(t, "noop", ins.memoryTriage.GetSessionID())

	// No-op memory triage methods should all return empty/nil without error.
	entities, err := ins.memoryTriage.AddRawText("some text")
	require.NoError(t, err)
	require.Empty(t, entities)

	results, err := ins.memoryTriage.SearchBySemantics("query", 5)
	require.NoError(t, err)
	require.Empty(t, results)

	tagResults, err := ins.memoryTriage.SearchByTags([]string{"tag"}, false, 5)
	require.NoError(t, err)
	require.Empty(t, tagResults)

	require.NoError(t, ins.memoryTriage.HandleMemory("anything"))

	smResult, err := ins.memoryTriage.SearchMemory("query", 1000)
	require.NoError(t, err)
	require.NotNil(t, smResult)
	require.Empty(t, smResult.Memories)

	noAIResult, err := ins.memoryTriage.SearchMemoryWithoutAI("query", 1000)
	require.NoError(t, err)
	require.NotNil(t, noAIResult)
	require.Empty(t, noAIResult.Memories)

	require.NoError(t, ins.memoryTriage.Close())

	// Ensure config also reflects the no-op.
	require.NotNil(t, ins.config.MemoryTriage)
	require.Equal(t, "noop", ins.config.MemoryTriage.GetSessionID())
}

// TestDisableMemoryTriage_FullLoop verifies that a complete ReAct loop
// (free input → directly answer → finish) works correctly when memory
// triage is disabled. The task should complete without any memory-related
// errors.
func TestDisableMemoryTriage_FullLoop(t *testing.T) {
	flag := ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// NewTestReAct already injects WithMemoryTriage(MockMemoryTriage) in its
	// basic options. Passing WithDisableMemoryTriage(true) after that should
	// override it with NoOp — this is the priority we want to verify.
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedFreeInputOutput(i, flag)
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithDisableMemoryTriage(true),
	)
	require.NoError(t, err)

	// Confirm memory triage is disabled (NoOp), even though NewTestReAct
	// injected a MockMemoryTriage earlier.
	require.Equal(t, "noop", ins.memoryTriage.GetSessionID())

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "abc",
		}
		close(in)
	}()

	after := time.After(15 * time.Second)
	haveResult := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.NodeId == "result" {
				result := jsonpath.FindFirst(e.GetContent(), "$..result")
				if utils.InterfaceToString(result) == flag || bytes.Contains([]byte(e.GetContent()), []byte(flag)) {
					haveResult = true
					break LOOP
				}
			}
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				status := utils.InterfaceToString(result)
				if status == "completed" || status == "failed" {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}

	require.True(t, haveResult, "Expected to have at least one result event containing the flag")
}

// TestDisableMemoryTriage_OverridesExplicitMemoryTriage verifies that
// DisableMemoryTriage=true takes priority over an explicitly provided
// MemoryTriage (e.g. via WithMemoryTriage). This ensures the disable
// flag is always authoritative.
func TestDisableMemoryTriage_OverridesExplicitMemoryTriage(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)

	mockMT := aimem.NewMockMemoryTriage()

	ins, err := NewReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedFreeInputOutput(i, "override-flag")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisablePerception(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableDynamicPlanning(true),
		aicommon.WithPeriodicVerificationInterval(0),
		aicommon.WithDisableIncreaseIteration(true),
		// Explicitly provide a mock memory triage...
		aicommon.WithMemoryTriage(mockMT),
		// ...but also disable memory triage. Disable should win.
		aicommon.WithDisableMemoryTriage(true),
	)
	require.NoError(t, err)

	// The instance should use no-op, not the mock.
	require.NotNil(t, ins.memoryTriage)
	require.Equal(t, "noop", ins.memoryTriage.GetSessionID())
}
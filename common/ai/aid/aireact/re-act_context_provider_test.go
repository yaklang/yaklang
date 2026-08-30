package aireact

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type inlineContextProbeStop struct{}

// Observe the real queue execution boundary before entering any model loop.
// A controlled panic also exercises cleanup when execution is interrupted.
type inlineContextProbeTask struct {
	aicommon.AIStatefulTask
	onProcessing func()
	interrupt    bool
}

func (task *inlineContextProbeTask) SetStatus(status aicommon.AITaskState) {
	task.AIStatefulTask.SetStatus(status)
	if status == aicommon.AITaskState_Processing {
		task.onProcessing()
		if task.interrupt {
			panic(inlineContextProbeStop{})
		}
	}
}

func newInlineContextQueueReAct(t *testing.T) *ReAct {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &ReAct{
		config: &aicommon.Config{
			Ctx: ctx, BaseCheckpointableStorage: &aicommon.BaseCheckpointableStorage{},
			ContextProviderManager: aicommon.NewContextProviderManager(),
			Timeline:               aicommon.NewTimeline(nil, nil),
		},
		taskQueue: NewTaskQueue("inline-context-fixture"),
	}
}

func inlineContextEvent(content ...string) *ypb.AIInputEvent {
	event := &ypb.AIInputEvent{IsFreeInput: true, FreeInput: "analyze the current input", FocusModeLoop: "inline-context-fixture"}
	for _, body := range content {
		event.AttachedResourceInfo = append(event.AttachedResourceInfo, &ypb.AttachedResourceInfo{
			Type: aicommon.CONTEXT_PROVIDER_TYPE_FILE, Key: aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT, Value: body,
		})
	}
	return event
}

func runInlineContextQueueProbe(t *testing.T, react *ReAct, observe func(), interrupt bool) {
	t.Helper()
	queued := react.taskQueue.GetFirst()
	if queued == nil {
		t.Fatal("inline context fixture has no queued task")
	}
	// The probe queue contains only this task; preserve the remaining inputs
	// so tasks enqueued during observation are checked on their own next turn.
	remaining := react.taskQueue
	probeQueue := NewTaskQueue("inline-context-probe")
	probeTask := &inlineContextProbeTask{AIStatefulTask: queued, onProcessing: observe, interrupt: interrupt}
	if !interrupt {
		// Recovery's missing-data exit returns normally without invoking AI.
		probeTask.SetTaskKind(aicommon.AITaskKind_Recovery)
	}
	if err := probeQueue.Append(probeTask); err != nil {
		t.Fatal(err)
	}
	// Observe incoming messages on the same real queue as the running task.
	for !remaining.IsEmpty() {
		if err := probeQueue.Append(remaining.GetFirst()); err != nil {
			t.Fatal(err)
		}
	}
	react.taskQueue = probeQueue
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				if _, expected := recovered.(inlineContextProbeStop); !expected || !interrupt {
					t.Fatalf("unexpected queue execution panic: %v", recovered)
				}
			} else if interrupt {
				t.Error("queue probe did not reach task activation")
			}
		}()
		react.processReActFromQueue()
	}()
	react.setCurrentTask(nil)
	probeTask.AIStatefulTask.SetStatus(aicommon.AITaskState_Aborted)
}

func TestReActInlineAttachmentContextTaskLifecycle(t *testing.T) {
	react := newInlineContextQueueReAct(t)
	manager := react.config.ContextProviderManager
	manager.Register("unrelated-provider", func(aicommon.AICallerConfigIf, *aicommon.Emitter, string) (string, error) {
		return "UNRELATED_CONTEXT", nil
	})
	path := filepath.Join(t.TempDir(), "reference.txt")
	if err := os.WriteFile(path, []byte("PATH_REFERENCE_CONTEXT"), 0o600); err != nil {
		t.Fatal(err)
	}
	longBody := "Filename: first.log\n" + strings.Repeat("INFO synthetic request\n", 100) + "FIRST_TAIL_ANOMALY"
	secondBody := "Filename: second.log\nSECOND_FILE_CONTEXT"
	event := inlineContextEvent(longBody, secondBody)
	event.AttachedFilePath = []string{path}
	event.AttachedResourceInfo = append(event.AttachedResourceInfo, nil, &ypb.AttachedResourceInfo{
		Type: aicommon.AttachedResourceTypeCode, Key: aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT, Value: "WRITABLE_DELIVERY_NOT_CONTEXT",
	})
	if err := react.handleFreeValue(event); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(manager.Execute(nil, nil), "FIRST_TAIL_ANOMALY") {
		t.Fatal("inline content became active while its input was only queued")
	}
	var active, afterQueued, afterEmpty, afterNested string
	runInlineContextQueueProbe(t, react, func() {
		active = manager.Execute(nil, nil)
		if err := react.handleFreeValue(inlineContextEvent("Filename: queued.log\nQUEUED_CONTEXT")); err != nil {
			t.Error(err)
		}
		afterQueued = manager.Execute(nil, nil)
		if err := react.handleFreeValue(inlineContextEvent()); err != nil {
			t.Error(err)
		}
		afterEmpty = manager.Execute(nil, nil)
		react.setCurrentTask(aicommon.NewStatefulTaskBase("nested-intent", "nested query", react.config.GetContext(), nil, true))
		afterNested = manager.Execute(nil, nil)
	}, true)
	for name, rendered := range map[string]string{"active": active, "queued attachment": afterQueued, "queued empty": afterEmpty, "nested task": afterNested} {
		for _, body := range []string{longBody, secondBody, "UNRELATED_CONTEXT", "PATH_REFERENCE_CONTEXT"} {
			if !strings.Contains(rendered, body) {
				t.Errorf("%s context lost current attachment/reference content", name)
			}
		}
		if strings.Contains(rendered, "QUEUED_CONTEXT") || strings.Contains(rendered, "WRITABLE_DELIVERY_NOT_CONTEXT") {
			t.Errorf("%s context contains data not owned by the running input", name)
		}
	}
	assertOnlyReferences := func(rendered string) {
		t.Helper()
		if strings.Contains(rendered, "FIRST_TAIL_ANOMALY") || strings.Contains(rendered, "SECOND_FILE_CONTEXT") || strings.Contains(rendered, "QUEUED_CONTEXT") {
			t.Error("completed input left stale inline context")
		}
		if !strings.Contains(rendered, "UNRELATED_CONTEXT") || !strings.Contains(rendered, "PATH_REFERENCE_CONTEXT") {
			t.Error("inline cleanup removed unrelated or path providers")
		}
	}
	assertOnlyReferences(manager.Execute(nil, nil))
	var next string
	runInlineContextQueueProbe(t, react, func() { next = manager.Execute(nil, nil) }, true)
	if !strings.Contains(next, "QUEUED_CONTEXT") || strings.Contains(next, "FIRST_TAIL_ANOMALY") || strings.Contains(next, "SECOND_FILE_CONTEXT") {
		t.Error("next queued input did not receive its own inline snapshot")
	}
	assertOnlyReferences(manager.Execute(nil, nil))
	runInlineContextQueueProbe(t, react, func() { assertOnlyReferences(manager.Execute(nil, nil)) }, true)
	assertOnlyReferences(manager.Execute(nil, nil))
}

func TestReActInlineAttachmentContextSessionIsolation(t *testing.T) {
	first, second := newInlineContextQueueReAct(t), newInlineContextQueueReAct(t)
	if err := first.handleFreeValue(inlineContextEvent("SESSION_ONE_ATTACHMENT")); err != nil {
		t.Fatal(err)
	}
	if err := second.handleFreeValue(inlineContextEvent("SESSION_TWO_ATTACHMENT")); err != nil {
		t.Fatal(err)
	}
	var firstDuringSecond, secondDuringFirst string
	runInlineContextQueueProbe(t, first, func() {
		runInlineContextQueueProbe(t, second, func() {
			firstDuringSecond = first.config.ContextProviderManager.Execute(nil, nil)
			secondDuringFirst = second.config.ContextProviderManager.Execute(nil, nil)
		}, true)
	}, true)
	if !strings.Contains(firstDuringSecond, "SESSION_ONE_ATTACHMENT") || strings.Contains(firstDuringSecond, "SESSION_TWO_ATTACHMENT") {
		t.Error("first session did not retain its own inline context")
	}
	if !strings.Contains(secondDuringFirst, "SESSION_TWO_ATTACHMENT") || strings.Contains(secondDuringFirst, "SESSION_ONE_ATTACHMENT") {
		t.Error("second session did not retain its own inline context")
	}
	if strings.Contains(first.config.ContextProviderManager.Execute(nil, nil), "SESSION_ONE_ATTACHMENT") || strings.Contains(second.config.ContextProviderManager.Execute(nil, nil), "SESSION_TWO_ATTACHMENT") {
		t.Error("finished sessions retained inline context")
	}
}

func TestReActInlineAttachmentContextCleansUpOnTaskReturn(t *testing.T) {
	react := newInlineContextQueueReAct(t)
	if err := react.handleFreeValue(inlineContextEvent("INLINE_TASK_RETURN_CONTEXT")); err != nil {
		t.Fatal(err)
	}
	var active string
	runInlineContextQueueProbe(t, react, func() { active = react.config.ContextProviderManager.Execute(nil, nil) }, false)
	if !strings.Contains(active, "INLINE_TASK_RETURN_CONTEXT") {
		t.Error("inline content was not visible before task execution")
	}
	if strings.Contains(react.config.ContextProviderManager.Execute(nil, nil), "INLINE_TASK_RETURN_CONTEXT") {
		t.Error("normally returning task retained inline context")
	}
}

func TestReActInlineAttachmentContextPreservesExistingProvider(t *testing.T) {
	for _, hasInline := range []bool{true, false} {
		react := newInlineContextQueueReAct(t)
		manager := react.config.ContextProviderManager
		manager.Register("free-input-inline-file-attachments", func(aicommon.AICallerConfigIf, *aicommon.Emitter, string) (string, error) {
			return "EXISTING_PROVIDER_CONTEXT", nil
		})
		event := inlineContextEvent()
		if hasInline {
			event = inlineContextEvent("CURRENT_INLINE_CONTEXT")
		}
		if err := react.handleFreeValue(event); err != nil {
			t.Fatal(err)
		}
		var active string
		runInlineContextQueueProbe(t, react, func() { active = manager.Execute(nil, nil) }, true)
		if !strings.Contains(active, "EXISTING_PROVIDER_CONTEXT") || !strings.Contains(manager.Execute(nil, nil), "EXISTING_PROVIDER_CONTEXT") {
			t.Errorf("task inline=%t overwrote or removed an unrelated provider with the same name", hasInline)
		}
		if hasInline && !strings.Contains(active, "CURRENT_INLINE_CONTEXT") {
			t.Error("existing provider prevented inline content from being registered")
		}
	}
}

func applyInlineContextConfigOptions(t *testing.T, opts []aicommon.ConfigOption) *aicommon.Config {
	t.Helper()
	// Apply the real inherited options without constructing an AI runtime or
	// loading a provider. The parent test context owns cancellation of children.
	child := &aicommon.Config{}
	for _, option := range opts {
		if err := option(child); err != nil {
			t.Fatal(err)
		}
	}
	if child.ContextProviderManager == nil {
		t.Fatal("derived config lost the context provider manager")
	}
	return child
}

func TestReActInlineAttachmentContextDerivedTaskSnapshot(t *testing.T) {
	for _, hasInline := range []bool{true, false} {
		name := "without_inline"
		if hasInline {
			name = "with_inline"
		}
		t.Run(name, func(t *testing.T) {
			react := newInlineContextQueueReAct(t)
			manager := react.config.ContextProviderManager
			var dynamic atomic.Value
			dynamic.Store("ORDINARY_DYNAMIC_INITIAL")
			manager.Register("ordinary-dynamic", func(aicommon.AICallerConfigIf, *aicommon.Emitter, string) (string, error) {
				return dynamic.Load().(string), nil
			})
			manager.Register("ordinary-removed", aicommon.FileContentContextProvider("ORDINARY_REMOVED"))
			event := inlineContextEvent()
			if hasInline {
				event = inlineContextEvent("TASK_A_INLINE")
			}
			if err := react.handleFreeValue(event); err != nil {
				t.Fatal(err)
			}
			var child *aicommon.Config
			var childOptions []aicommon.ConfigOption
			runInlineContextQueueProbe(t, react, func() {
				// Async P&E/Forge capture these options before their startup
				// barrier releases the root, but may use them after it returns.
				childOptions = aicommon.ConvertConfigToOptions(react.config)
				child = applyInlineContextConfigOptions(t, childOptions)
			}, false)
			if react.config.ContextProviderManager != manager {
				t.Fatal("derivation replaced the running root manager")
			}
			assertChildInline := func(label, rendered string) {
				t.Helper()
				if strings.Contains(rendered, "TASK_A_INLINE") != hasInline || strings.Contains(rendered, "TASK_B_INLINE") {
					t.Errorf("%s did not retain task A's immutable inline snapshot (inline=%t)", label, hasInline)
				}
			}
			assertChildInline("after root A returned", child.ContextProviderManager.Execute(nil, nil))
			if strings.Contains(manager.Execute(nil, nil), "TASK_A_INLINE") {
				t.Error("root A retained inline context after returning")
			}
			if err := react.handleFreeValue(inlineContextEvent("TASK_B_INLINE")); err != nil {
				t.Fatal(err)
			}
			runInlineContextQueueProbe(t, react, func() {
				// Delayed option application must not capture the now-current B.
				delayed := applyInlineContextConfigOptions(t, childOptions)
				grandchild := applyInlineContextConfigOptions(t, aicommon.ConvertConfigToOptions(child))
				dynamic.Store("ORDINARY_DYNAMIC_UPDATED")
				manager.Unregister("ordinary-removed")
				manager.Register("ordinary-added", aicommon.FileContentContextProvider("ORDINARY_ADDED"))
				child.ContextProviderManager.Register("ordinary-child-added", aicommon.FileContentContextProvider("ORDINARY_CHILD_ADDED"))
				for label, config := range map[string]*aicommon.Config{"child": child, "delayed child": delayed, "grandchild": grandchild} {
					// Read from a continuing child after A has returned and B has
					// started; no model, Provider, or scheduling override is needed.
					result := make(chan string, 1)
					go func() { result <- config.ContextProviderManager.Execute(nil, nil) }()
					rendered := <-result
					assertChildInline(label, rendered)
					for _, expected := range []string{"ORDINARY_DYNAMIC_UPDATED", "ORDINARY_ADDED", "ORDINARY_CHILD_ADDED"} {
						if !strings.Contains(rendered, expected) {
							t.Errorf("%s lost live ordinary provider %s", label, expected)
						}
					}
					if strings.Contains(rendered, "ORDINARY_REMOVED") || strings.Contains(rendered, "ORDINARY_DYNAMIC_INITIAL") {
						t.Errorf("%s froze ordinary providers instead of only inline input", label)
					}
				}
				parentContext := manager.Execute(nil, nil)
				if !strings.Contains(parentContext, "TASK_B_INLINE") || strings.Contains(parentContext, "TASK_A_INLINE") || !strings.Contains(parentContext, "ORDINARY_CHILD_ADDED") {
					t.Error("root B lost its own inline or shared ordinary providers")
				}
				grandchild.ContextProviderManager.Unregister("ordinary-child-added")
				if strings.Contains(manager.Execute(nil, nil), "ORDINARY_CHILD_ADDED") {
					t.Error("derived manager no longer shares ordinary provider removal")
				}
			}, true)
			assertChildInline("after root B returned", child.ContextProviderManager.Execute(nil, nil))
		})
	}
}

func TestReActInlineAttachmentContextDerivedTaskBudget(t *testing.T) {
	react := newInlineContextQueueReAct(t)
	manager := react.config.ContextProviderManager
	ordinary := strings.Repeat("ordinary ", 26000)
	inline := strings.Repeat("attached ", 26000) + "BUDGET_INLINE_TAIL"
	if aicommon.MeasureTokens(ordinary) >= 48*1024 || aicommon.MeasureTokens(inline) >= 48*1024 || aicommon.MeasureTokens(ordinary+inline) <= 48*1024 {
		t.Fatal("budget fixture must exceed one shared budget but fit two separate budgets")
	}
	manager.Register("ordinary-budget", aicommon.FileContentContextProvider(ordinary))
	if err := react.handleFreeValue(inlineContextEvent(inline)); err != nil {
		t.Fatal(err)
	}
	var child *aicommon.Config
	runInlineContextQueueProbe(t, react, func() {
		child = applyInlineContextConfigOptions(t, aicommon.ConvertConfigToOptions(react.config))
	}, false)
	rendered := child.ContextProviderManager.Execute(nil, nil)
	if aicommon.MeasureTokens(rendered) > 48*1024 || !strings.Contains(rendered, "...") {
		t.Error("derived ordinary and inline providers did not share the existing 48k token budget")
	}
	if !strings.Contains(rendered, "BUDGET_INLINE_TAIL") {
		t.Error("derived budgeted context lost task A's inline snapshot")
	}
}

func TestReAct_AllContextProviders(t *testing.T) {
	t.Skip("skipping context provider test, because it is removed")
	// 1. Setup test file
	testFileContent := "UniqueFileContent_" + ksuid.New().String()
	tempFile, err := os.CreateTemp("", "test_all_ctx_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	if _, err := tempFile.WriteString(testFileContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tempFile.Close()

	// 2. Setup knowledge base
	kbName := "test_kb_all_" + ksuid.New().String()
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()
	ragSystem, err := rag.Get(kbName, rag.WithEmbeddingClient(mockEmbedding))
	if err != nil {
		t.Fatalf("Failed to create rag system: %v", err)
	}
	ragSystem.Add("test", mockEmbedding.GenerateRandomText(10))
	defer func() {
		rag.DeleteRAG(consts.GetGormProfileDatabase(), kbName)
	}()

	// 3. Setup AIForge
	testForgeName := "test_forge_all_" + ksuid.New().String()
	forge := &schema.AIForge{
		ForgeName:   testForgeName,
		Description: "Test forge for all context providers",
		ForgeType:   "json",
	}
	err = yakit.CreateAIForge(consts.GetGormProfileDatabase(), forge)
	if err != nil {
		t.Fatalf("Failed to create AIForge: %v", err)
	}
	defer func() {
		yakit.DeleteAIForgeByName(consts.GetGormProfileDatabase(), testForgeName)
	}()

	// 4. Setup test tool
	testToolName := "test_tool_all_" + ksuid.New().String()
	testTool, err := aitool.New(
		testToolName,
		aitool.WithDescription("Test tool for all context providers"),
		aitool.WithStringParam("input"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "test", nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create test tool: %v", err)
	}

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	var promptReceived string
	var promptMutex sync.Mutex
	fileFound := false
	kbFound := false
	forgeFound := false
	toolFound := false

	reactIns, err := NewTestReAct(
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			promptMutex.Lock()
			promptReceived = req.GetPrompt()
			if utils.MatchAllOfSubString(promptReceived, testFileContent) {
				fileFound = true
			}
			if utils.MatchAllOfSubString(promptReceived, "Knowledge Base Info", kbName) {
				kbFound = true
			}
			if utils.MatchAllOfSubString(promptReceived, "AIForge Info", testForgeName) {
				forgeFound = true
			}
			if utils.MatchAllOfSubString(promptReceived, "AITool Info", testToolName) {
				toolFound = true
			}
			promptMutex.Unlock()

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// Add test tool to manager
	err = reactIns.config.GetAiToolManager().AppendTools(testTool)
	if err != nil {
		t.Fatalf("Failed to append test tool: %v", err)
	}

	// Send input event with all AttachedResourceInfo types
	// Key 是内部常量（如 file_path, name），Value 是实际值
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "Please use all attached resources",
			AttachedResourceInfo: []*ypb.AttachedResourceInfo{
				{
					Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
					Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
					Value: tempFile.Name(),
				},
				{
					Key:   aicommon.CONTEXT_PROVIDER_KEY_NAME,
					Type:  aicommon.CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE,
					Value: kbName,
				},
				{
					Key:   aicommon.CONTEXT_PROVIDER_KEY_NAME,
					Type:  aicommon.CONTEXT_PROVIDER_TYPE_AIFORGE,
					Value: testForgeName,
				},
				{
					Key:   aicommon.CONTEXT_PROVIDER_KEY_NAME,
					Type:  aicommon.CONTEXT_PROVIDER_TYPE_AITOOL,
					Value: testToolName,
				},
			},
		}
		close(in)
	}()

	// Wait for AI callback to be triggered
	after := time.After(10 * time.Second)
	allFound := false

LOOP:
	for {
		select {
		case <-out:
			promptMutex.Lock()
			allFound = fileFound && kbFound && forgeFound && toolFound
			promptMutex.Unlock()
			if allFound {
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}

	promptMutex.Lock()
	defer promptMutex.Unlock()

	// Check each context provider result
	if !fileFound {
		t.Errorf("File content not found in prompt. Looking for: %s", testFileContent)
	}
	if !kbFound {
		t.Errorf("Knowledge base info not found in prompt. Looking for: 'Knowledge Base Info' and '%s'", kbName)
	}
	if !forgeFound {
		t.Errorf("AIForge info not found in prompt. Looking for: 'AIForge Info' and '%s'", testForgeName)
	}
	if !toolFound {
		t.Errorf("AITool info not found in prompt. Looking for: 'AITool Info' and '%s'", testToolName)
	}

	if !allFound {
		t.Fatalf("Not all context providers were found in prompt.\nFile: %v, KB: %v, Forge: %v, Tool: %v\nPrompt (first 3000 chars): %s",
			fileFound, kbFound, forgeFound, toolFound, utils.ShrinkString(promptReceived, 3000))
	}

	t.Logf("All context providers via input channel test passed")
}

func TestReAct_ContextProvider_ErrorHandling(t *testing.T) {
	t.Skip("skipping context provider error handling test, because it is removed")
	t.Run("FileNotExist_ViaInputChannel", func(t *testing.T) {
		nonExistentFile := "/tmp/non_existent_file_" + ksuid.New().String() + ".txt"

		in := make(chan *ypb.AIInputEvent, 10)
		out := make(chan *ypb.AIOutputEvent, 100)

		var promptReceived string
		var promptMutex sync.Mutex
		errorInPrompt := false

		_, err := NewTestReAct(
			aicommon.WithEventInputChan(in),
			aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
				out <- e.ToGRPC()
			}),
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				promptMutex.Lock()
				promptReceived = req.GetPrompt()
				if utils.MatchAllOfSubString(promptReceived, "Error getting context") {
					errorInPrompt = true
				}
				promptMutex.Unlock()

				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
		)
		if err != nil {
			t.Fatalf("Failed to create ReAct instance: %v", err)
		}

		go func() {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "Please read this file",
				AttachedResourceInfo: []*ypb.AttachedResourceInfo{
					{
						Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
						Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
						Value: nonExistentFile,
					},
				},
			}
			close(in)
		}()

		after := time.After(10 * time.Second)

	LOOP:
		for {
			select {
			case <-out:
				if errorInPrompt {
					break LOOP
				}
			case <-after:
				break LOOP
			}
		}

		promptMutex.Lock()
		defer promptMutex.Unlock()

		if !errorInPrompt {
			t.Fatalf("Error message should be in prompt for non-existent file.\nPrompt (first 2000 chars): %s",
				utils.ShrinkString(promptReceived, 2000))
		}

		t.Logf("FileNotExist error handling via input channel test passed")
	})

	t.Run("UnknownType_ViaInputChannel", func(t *testing.T) {
		in := make(chan *ypb.AIInputEvent, 10)
		out := make(chan *ypb.AIOutputEvent, 100)

		var promptReceived string
		var promptMutex sync.Mutex
		errorInPrompt := false

		_, err := NewTestReAct(
			aicommon.WithEventInputChan(in),
			aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
				out <- e.ToGRPC()
			}),
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				promptMutex.Lock()
				promptReceived = req.GetPrompt()
				if utils.MatchAllOfSubString(promptReceived, "Error getting context", "unknown context provider type") {
					errorInPrompt = true
				}
				promptMutex.Unlock()

				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
		)
		if err != nil {
			t.Fatalf("Failed to create ReAct instance: %v", err)
		}

		go func() {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "Please use this unknown resource",
				AttachedResourceInfo: []*ypb.AttachedResourceInfo{
					{
						Key:  "some_key",
						Type: "unknown_type",
					},
				},
			}
			close(in)
		}()

		after := time.After(10 * time.Second)

	LOOP:
		for {
			select {
			case <-out:
				if errorInPrompt {
					break LOOP
				}
			case <-after:
				break LOOP
			}
		}

		promptMutex.Lock()
		defer promptMutex.Unlock()

		if !errorInPrompt {
			t.Fatalf("Error message should be in prompt for unknown type.\nPrompt (first 2000 chars): %s",
				utils.ShrinkString(promptReceived, 2000))
		}

		t.Logf("UnknownType error handling via input channel test passed")
	})
}

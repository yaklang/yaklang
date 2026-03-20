package reactloops

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type runtimeEnvironmentMockInvoker struct {
	*mock.MockInvoker
}

func (m *runtimeEnvironmentMockInvoker) GetYakExecutablePath() string {
	return "/usr/local/bin/yak"
}

func (m *runtimeEnvironmentMockInvoker) GetAISkillsInstallDir() string {
	return "/tmp/ai-skills"
}

func (m *runtimeEnvironmentMockInvoker) GetBuiltinAISkillsInstallDir() string {
	return "/tmp/ai-skills/builtin"
}

func (m *runtimeEnvironmentMockInvoker) GetAISkillsScannedDirs() []string {
	return []string{
		"/tmp/ai-skills",
		"/tmp/project/.cursor/skills",
	}
}

func TestGenerateLoopPrompt_IncludesRuntimeEnvironmentPaths(t *testing.T) {
	invoker := &runtimeEnvironmentMockInvoker{MockInvoker: mock.NewMockInvoker(context.Background())}

	loop := &ReActLoop{
		invoker:                    invoker,
		config:                     invoker.GetConfig(),
		emitter:                    invoker.GetConfig().GetEmitter(),
		loopName:                   "runtime-environment-test",
		maxIterations:              100,
		actions:                    omap.NewEmptyOrderedMap[string, *LoopAction](),
		loopActions:                omap.NewEmptyOrderedMap[string, LoopActionFactory](),
		streamFields:               omap.NewEmptyOrderedMap[string, *LoopStreamField](),
		aiTagFields:                omap.NewEmptyOrderedMap[string, *LoopAITagField](),
		vars:                       omap.NewEmptyOrderedMap[string, any](),
		taskMutex:                  new(sync.Mutex),
		currentMemories:            omap.NewEmptyOrderedMap[string, *aicommon.MemoryEntity](),
		memorySizeLimit:            10 * 1024,
		enableSelfReflection:       true,
		historySatisfactionReasons: make([]*SatisfactionRecord, 0),
		actionHistory:              make([]*ActionRecord, 0),
		actionHistoryMutex:         new(sync.Mutex),
		extraCapabilities:          NewExtraCapabilitiesManager(),
	}
	loop.actions.Set(loopAction_DirectlyAnswer.ActionType, loopAction_DirectlyAnswer)
	loop.actions.Set(loopAction_Finish.ActionType, loopAction_Finish)

	emitter := aicommon.NewEmitter("runtime-environment-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	task := aicommon.NewStatefulTaskBase("runtime-environment-task", "runtime environment prompt test", context.Background(), emitter, true)
	loop.SetCurrentTask(task)

	operator := NewActionHandlerOperator(task)
	prompt, err := loop.generateLoopPrompt("runtime_nonce", "test query", "", operator)
	if err != nil {
		t.Fatalf("failed to generate loop prompt: %v", err)
	}

	expectedSnippets := []string{
		"<|RUNTIME_ENVIRONMENT_runtime_nonce|>",
		"Yak executable absolute path: /usr/local/bin/yak",
		"AI skills installation directory: /tmp/ai-skills",
		"Built-in skills installation directory: /tmp/ai-skills/builtin",
		"/tmp/project/.cursor/skills",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("expected prompt to contain %q, but it did not.\nprompt:\n%s", snippet, prompt)
		}
	}
}

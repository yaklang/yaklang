package aiengine

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
)

type platformProfile struct {
	tools    []*aitool.Tool
	callback aicommon.AICallbackType
}

// WithPlatformOnlyProfile selects an immutable capability boundary. General
// engine options cannot widen this profile, override its callback, or enable
// persistence, code execution, MCP, skills, planning, or built-in tools.
func WithPlatformOnlyProfile(callback aicommon.AICallbackType, tools ...*aitool.Tool) AIEngineConfigOption {
	return func(c *AIEngineConfig) {
		c.platformProfile = &platformProfile{append([]*aitool.Tool(nil), tools...), callback}
	}
}

func platformOptions(ctx context.Context, c *AIEngineConfig, output chan *schema.AiOutputEvent) []aicommon.ConfigOption {
	callback := c.platformProfile.callback
	if callback == nil {
		callback = func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("platform model callback required")
		}
	}
	manager := buildinaitools.NewToolManagerByToolGetter(func() []*aitool.Tool { return nil }, buildinaitools.WithOnlyTools(c.platformProfile.tools...))
	iterations := c.MaxIteration
	if iterations <= 0 || iterations > 12 {
		iterations = 12
	}
	return []aicommon.ConfigOption{
		aicommon.WithContext(ctx), aicommon.WithAICallback(callback),
		func(c *aicommon.Config) error {
			c.DisableInputDirectives = true
			c.DisableGlobalPreset = true
			c.DisableHelperCoordinators = true
			return nil
		},
		aicommon.WithToolManager(manager), aicommon.WithEphemeralStorage(),
		aicommon.WithPersistentSessionId(""), aicommon.WithMemoryTriageId(""),
		aicommon.WithMemoryTriage(aicommon.NewNoOpMemoryTriage()),
		aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
		aicommon.WithEnablePlanAndExec(false), aicommon.WithEnablePETaskAnalyze(false),
		aicommon.WithAllowRequireForUserInteract(false), aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithDisablePerception(true), aicommon.WithDisableIntentRecognition(true), aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableEnhanceDirectlyAnswer(true), aicommon.WithGenerateReport(false),
		aicommon.WithWorkdir("/"), aicommon.WithMaxIterationCount(int64(iterations)),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithReActActionPolicy(func(_ string, action string) bool {
			switch action {
			case "directly_answer", "finish", "require_tool", "directly_call_tool":
				return true
			}
			return false
		}),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case output <- e:
			case <-ctx.Done():
			}
		}),
	}
}

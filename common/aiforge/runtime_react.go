package aiforge

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// RuntimeForgeReActPreparation contains the configuration that must be applied
// to a long-lived ReAct session before it receives its first user event.
type RuntimeForgeReActPreparation struct {
	Options []aicommon.ConfigOption
}

type RuntimeForgeReActPreparer func(
	ctx context.Context,
	params []*ypb.ExecParamItem,
) (*RuntimeForgeReActPreparation, error)

// WithToolScope confines a Forge to the tools it explicitly owns. This is more
// restrictive than merely suggesting tool names: built-in filesystem, search,
// profile, and MCP tools are removed from the enabled inventory.
func WithToolScope(tools ...*aitool.Tool) aicommon.ConfigOption {
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool != nil && tool.Name != "" {
			toolNames = append(toolNames, tool.Name)
		}
	}
	return aicommon.WithAiToolManagerOptions(func(manager *buildinaitools.AiToolManager) {
		manager.RestrictToTools(toolNames...)
	})
}

// PrepareRuntimeForgeReAct adapts a ForgeBlueprint to the streaming ReAct
// transport. Runtime Forges are already selected by the caller, so generic
// intent discovery, auto-loaded skills, MCP tools, and sub-agent dispatch would
// only create alternate paths around the Forge's bound runtime capabilities.
func PrepareRuntimeForgeReAct(
	forge *ForgeBlueprint,
	params []*ypb.ExecParamItem,
	options ...aicommon.ConfigOption,
) (*RuntimeForgeReActPreparation, error) {
	if forge == nil {
		return nil, utils.Error("runtime forge blueprint is nil")
	}
	firstPrompt, generatedOptions, err := forge.GenerateFirstPromptWithMemoryOption(params)
	if err != nil {
		return nil, err
	}

	finalOptions := []aicommon.ConfigOption{
		aicommon.WithForgeName(forge.Name),
		aicommon.WithPlanPrompt(firstPrompt),
	}
	finalOptions = append(finalOptions, generatedOptions...)
	finalOptions = append(finalOptions,
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisablePerception(true),
		aicommon.WithEnableDispatchSubReactAgent(false),
	)
	finalOptions = append(finalOptions, options...)
	finalOptions = append(finalOptions, WithToolScope(forge.Tools...))

	return &RuntimeForgeReActPreparation{Options: finalOptions}, nil
}

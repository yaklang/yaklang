package browserauthorization

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const ForgeName = "browser_authorization_analysis"

//go:embed prompts/init.txt
var initializePrompt string

//go:embed prompts/persistent.txt
var persistentPrompt string

type Runner struct {
	service Service
}

func authorizationToolScopeOption(tools []*aitool.Tool) aicommon.ConfigOption {
	return aiforge.WithToolScope(tools...)
}

func NewRunner(service Service) *Runner {
	return &Runner{service: service}
}

func NewExecutor(service Service) aiforge.ForgeExecutor {
	return NewRunner(service).Execute
}

func (r *Runner) prepare(
	ctx context.Context,
	items []*ypb.ExecParamItem,
) (*Config, *aiforge.ForgeBlueprint, []*aitool.Tool, error) {
	if r == nil || r.service == nil || !r.service.Available() {
		return nil, nil, nil, errors.New("browser authorization workspace service is not running")
	}
	config, err := ParseConfig(items)
	if err != nil {
		return nil, nil, nil, err
	}
	if _, err := inspectBoundWorkspace(ctx, r.service, config.WorkspaceID); err != nil {
		return nil, nil, nil, fmt.Errorf("inspect browser authorization workspace: %w", err)
	}
	tools, err := buildTools(r.service, config.WorkspaceID)
	if err != nil {
		return nil, nil, nil, err
	}
	forge := aiforge.NewForgeBlueprint(
		ForgeName,
		aiforge.WithInitializePrompt(initializePrompt),
		aiforge.WithPersistentPrompt(persistentPrompt),
		aiforge.WithTools(tools...),
	)
	return &config, forge, tools, nil
}

func (r *Runner) PrepareReAct(
	ctx context.Context,
	items []*ypb.ExecParamItem,
) (*aiforge.RuntimeForgeReActPreparation, error) {
	config, forge, _, err := r.prepare(ctx, items)
	if err != nil {
		return nil, err
	}
	return aiforge.PrepareRuntimeForgeReAct(forge, []*ypb.ExecParamItem{
		{Key: "query", Value: config.Query},
		{Key: "workspace_id", Value: config.WorkspaceID},
	})
}

func (r *Runner) Execute(
	ctx context.Context,
	items []*ypb.ExecParamItem,
	options ...aicommon.ConfigOption,
) (*aiforge.ForgeResult, error) {
	config, forge, tools, err := r.prepare(ctx, items)
	if err != nil {
		return nil, err
	}
	scopedOptions := append([]aicommon.ConfigOption{}, options...)
	scopedOptions = append(scopedOptions, authorizationToolScopeOption(tools))
	coordinator, err := forge.CreateCoordinator(ctx, []*ypb.ExecParamItem{
		{Key: "query", Value: config.Query},
		{Key: "workspace_id", Value: config.WorkspaceID},
	}, scopedOptions...)
	if err != nil {
		return nil, err
	}
	if err := coordinator.Run(); err != nil {
		return nil, err
	}
	return &aiforge.ForgeResult{Forge: forge}, nil
}

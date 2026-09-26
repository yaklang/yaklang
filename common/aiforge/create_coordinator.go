package aiforge

import (
	"context"
	"slices"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (t *ForgeBlueprint) CreateCoordinatorWithQuery(ctx context.Context, originQuery string, opts ...aicommon.ConfigOption) (*aid.Coordinator, error) {
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOptionWithQuery(originQuery)
	if err != nil {
		return nil, err
	}
	return t.createCoordinatorWithRenderedPrompt(ctx, firstQuery, extraOpts, opts...)
}

// CreateCoordinatorWithQueryAndParams binds the server-authored user query
// and the already validated application parameters into one immutable Forge
// invocation. Unlike the legacy DB-backed entrypoint, it does not resolve or
// execute parameter code.
func (t *ForgeBlueprint) CreateCoordinatorWithQueryAndParams(
	ctx context.Context,
	originQuery string,
	params []*ypb.ExecParamItem,
	opts ...aicommon.ConfigOption,
) (*aid.Coordinator, error) {
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOptionWithQueryAndParams(originQuery, params)
	if err != nil {
		return nil, err
	}
	return t.createCoordinatorWithRenderedPrompt(ctx, firstQuery, extraOpts, opts...)
}

func (t *ForgeBlueprint) createCoordinatorWithRenderedPrompt(
	ctx context.Context,
	firstQuery string,
	extraOpts []aicommon.ConfigOption,
	opts ...aicommon.ConfigOption,
) (*aid.Coordinator, error) {
	extraOpts = append(extraOpts, aicommon.WithForgeName(t.Name))
	extraOpts = append(extraOpts, opts...)

	finalOpts := slices.Clone(t.AIOptions)
	finalOpts = append(finalOpts, extraOpts...)
	return aid.NewCoordinatorContext(ctx, firstQuery, finalOpts...)
}

func (t *ForgeBlueprint) CreateCoordinator(ctx context.Context, i any, opts ...aicommon.ConfigOption) (*aid.Coordinator, error) {
	params := Any2ExecParams(i)
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOption(params)
	if err != nil {
		return nil, err
	}

	rawInput := ExecParams2PromptString(params)
	finalOpts := slices.Clone(t.AIOptions)
	finalOpts = append(finalOpts, []aicommon.ConfigOption{
		aicommon.WithForgeName(t.Name),
		aicommon.WithPlanPrompt(firstQuery),
	}...)
	finalOpts = append(finalOpts, extraOpts...)
	finalOpts = append(finalOpts, opts...)

	return aid.NewCoordinatorContext(ctx, rawInput, finalOpts...)
}

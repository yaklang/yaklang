package aiforge

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
)

func (t *ForgeBlueprint) CreateCoordinatorWithQuery(ctx context.Context, originQuery string, opts ...aid.Option) (*aid.Coordinator, error) {
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOptionWithQuery(originQuery)
	if err != nil {
		return nil, err
	}
	extraOpts = append(extraOpts, aid.WithForgeName(t.Name))
	extraOpts = append(extraOpts, opts...)
	return aid.NewCoordinatorContext(ctx, firstQuery, extraOpts...)
}

func (t *ForgeBlueprint) CreateCoordinator(ctx context.Context, i any, opts ...aid.Option) (*aid.Coordinator, error) {
	params := Any2ExecParams(i)
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOption(params)
	if err != nil {
		return nil, err
	}
	extraOpts = append(extraOpts, aid.WithForgeParams(params))
	extraOpts = append(extraOpts, opts...)
	finalOpts := []aid.Option{aid.WithMemory(aid.GetDefaultMemory()), aid.WithForgeName(t.Name)}
	finalOpts = append(finalOpts, extraOpts...)
	return aid.NewCoordinatorContext(ctx, firstQuery, finalOpts...)
}

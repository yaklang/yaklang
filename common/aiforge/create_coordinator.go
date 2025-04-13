package aiforge

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (t *ForgeBlueprint) CreateCoordinatorWithQuery(ctx context.Context, originQuery string, opts ...aid.Option) (*aid.Coordinator, error) {
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOptionWithQuery(originQuery)
	if err != nil {
		return nil, err
	}
	extraOpts = append(extraOpts, opts...)
	return aid.NewCoordinatorContext(ctx, firstQuery, extraOpts...)
}

func (t *ForgeBlueprint) CreateCoordinator(ctx context.Context, params []*ypb.ExecParamItem, opts ...aid.Option) (*aid.Coordinator, error) {
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOption(params)
	if err != nil {
		return nil, err
	}
	extraOpts = append(extraOpts, opts...)
	return aid.NewCoordinatorContext(ctx, firstQuery, extraOpts...)
}

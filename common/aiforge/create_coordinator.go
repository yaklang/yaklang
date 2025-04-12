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
	extraOpts = append(extraOpts, opts...)
	return aid.NewCoordinatorContext(ctx, firstQuery, extraOpts...)
}

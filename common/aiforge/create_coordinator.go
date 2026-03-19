package aiforge

import (
	"context"
	"slices"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func (t *ForgeBlueprint) CreateCoordinatorWithQuery(ctx context.Context, originQuery string, opts ...aicommon.ConfigOption) (*aid.Coordinator, error) {
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOptionWithQuery(originQuery)
	if err != nil {
		return nil, err
	}

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
	// Inject each CLI param into the config key-value store so that loops can
	// retrieve them via config.GetConfigString("project_path") etc.
	for _, p := range params {
		if p.Key != "" && p.Value != "" {
			k, v := p.Key, p.Value
			finalOpts = append(finalOpts, func(c *aicommon.Config) error {
				c.SetConfig(k, v)
				return nil
			})
		}
	}
	finalOpts = append(finalOpts, extraOpts...)
	finalOpts = append(finalOpts, opts...)

	return aid.NewCoordinatorContext(ctx, rawInput, finalOpts...)
}

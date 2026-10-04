package aiforge

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (t *ForgeBlueprint) CreateCoordinatorWithQuery(ctx context.Context, originQuery string, opts ...aicommon.ConfigOption) (*ForgeExecution, error) {
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOptionWithQuery(originQuery)
	if err != nil {
		return nil, err
	}
	e, err := t.createCoordinatorWithRenderedPrompt(ctx, originQuery, firstQuery, extraOpts, opts...)
	if err == nil {
		e.query = originQuery
		e.resultParams, err = t.Params(originQuery, &ypb.ExecParamItem{Key: "query", Value: originQuery})
		if err != nil {
			e.Close()
			return nil, err
		}
	}
	return e, err
}

// CreateCoordinatorWithQueryAndParams renders the query and caller-validated
// parameters without interpreting CLI declarations. Templates decide where
// invocation data appears; callers own any additional prompt composition.
func (t *ForgeBlueprint) CreateCoordinatorWithQueryAndParams(
	ctx context.Context,
	originQuery string,
	params []Parameter,
	opts ...aicommon.ConfigOption,
) (*ForgeExecution, error) {
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOptionWithQueryAndParams(originQuery, params)
	if err != nil {
		return nil, err
	}
	e, err := t.createCoordinatorWithRenderedPrompt(ctx, originQuery, firstQuery, extraOpts, opts...)
	if err == nil {
		e.query = originQuery
		e.resultParams = t.promptParams(originQuery, params)
	}
	return e, err
}

func (t *ForgeBlueprint) createCoordinatorWithRenderedPrompt(
	ctx context.Context,
	query string,
	firstQuery string,
	extraOpts []aicommon.ConfigOption,
	opts ...aicommon.ConfigOption,
) (*ForgeExecution, error) {
	extraOpts = append(extraOpts, aicommon.WithPlanPrompt(firstQuery), aicommon.WithForgeName(t.Name))
	extraOpts = append(extraOpts, opts...)

	return t.newExecution(ctx, query, extraOpts)
}

func (t *ForgeBlueprint) CreateCoordinator(ctx context.Context, i any, opts ...aicommon.ConfigOption) (*ForgeExecution, error) {
	params := Any2ExecParams(i)
	firstQuery, extraOpts, err := t.GenerateFirstPromptWithMemoryOption(params)
	if err != nil {
		return nil, err
	}

	extraOpts = append(extraOpts, aicommon.WithPlanPrompt(firstQuery), aicommon.WithForgeName(t.Name))
	extraOpts = append(extraOpts, opts...)
	e, err := t.newExecution(ctx, ExecParams2PromptString(params), extraOpts)
	if err == nil {
		for _, p := range params {
			if p != nil && p.Key == "query" {
				e.query = p.Value
				break
			}
		}
		e.resultParams, err = t.Params(e.query, params...)
		if err != nil {
			e.Close()
			return nil, err
		}
	}
	return e, err
}

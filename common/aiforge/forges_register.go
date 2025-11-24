package aiforge

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ForgeExecutor func(context.Context, []*ypb.ExecParamItem, ...aicommon.ConfigOption) (*ForgeResult, error)

var forgeMutex = new(sync.RWMutex)
var forges = make(map[string]ForgeExecutor)

type ForgeResult struct {
	*aicommon.Action
	Formated any
	Forge    *ForgeBlueprint
}

func RegisterYakAiForge(cfg *YakForgeBlueprintConfig) error {
	blueprint, err := cfg.Build()
	if err != nil {
		return err
	}
	return RegisterForgeExecutor(cfg.Name, func(ctx context.Context, items []*ypb.ExecParamItem, opts ...aicommon.ConfigOption) (*ForgeResult, error) {
		ins, err := blueprint.CreateCoordinator(ctx, items, opts...)
		if err != nil {
			return nil, err
		}
		if err := ins.Run(); err != nil {
			return nil, err
		}
		return cfg.ForgeResult, nil
	})
}

func RegisterLiteForge(i string, params ...LiteForgeOption) error {
	lf, err := NewLiteForge(i, params...)
	if err != nil {
		return utils.Errorf("build lite forge failed: %v", err)
	}
	return RegisterForgeExecutor(i, lf.Execute)
}

func RegisterAIDBuildInForge(i string, params ...LiteForgeOption) error {
	lf, err := NewLiteForge(i, params...)
	if err != nil {
		return utils.Errorf("build lite forge failed: %v", err)
	}
	return aid.RegisterAIDBuildinForge(i, func(c context.Context, params []*ypb.ExecParamItem, opts ...aicommon.ConfigOption) (*aicommon.Action, error) {
		result, err := lf.Execute(c, params, opts...)
		if err != nil {
			return nil, err
		}
		return result.Action, nil
	})
}

func RegisterForgeExecutor(i string, f ForgeExecutor) error {
	forgeMutex.Lock()
	if _, ok := forges[i]; ok {
		forgeMutex.Unlock()
		return utils.Errorf("forge %s already registered", i)
	}
	forges[i] = f
	forgeMutex.Unlock()
	return nil
}

func ExecuteForge(
	forgeName string,
	ctx context.Context,
	params []*ypb.ExecParamItem,
	opts ...aicommon.ConfigOption,
) (*ForgeResult, error) {
	forgeMutex.RLock()
	defer forgeMutex.RUnlock()

	if forge, ok := forges[forgeName]; ok {
		return forge(ctx, params, opts...)
	} else {
		return nil, utils.Errorf("forge %s not found", forgeName)
	}
}

func convertForgeResultIntoCommonForgeResult(fr *ForgeResult) *aicommon.ForgeResult {
	return &aicommon.ForgeResult{
		Action: fr.Action,
		Name:   fr.Forge.Name,
	}
}

func init() {
	aicommon.RegisterPresetForgeExecuteCallback(func(name string, ctx context.Context, params any, opts ...aicommon.ConfigOption) (*aicommon.ForgeResult, error) {
		var finalParams []*ypb.ExecParamItem
		switch paramIns := params.(type) {
		case []*ypb.ExecParamItem:
			finalParams = paramIns
		default:
			finalParams = []*ypb.ExecParamItem{
				{Key: "query", Value: utils.InterfaceToString(params)},
			}
		}
		result, err := ExecuteForge(name, ctx, finalParams, opts...)
		if err != nil {
			return nil, err
		}
		return convertForgeResultIntoCommonForgeResult(result), nil
	})
}

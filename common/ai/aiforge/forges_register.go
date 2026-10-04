package aiforge

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/liteforge/liteforgeapp"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ForgeExecutor func(context.Context, []*ypb.ExecParamItem, ...aicommon.ConfigOption) (*ForgeResult, error)

var forgeMutex = new(sync.RWMutex)
var forges = make(map[string]ForgeExecutor)
var yakForges = make(map[string]ForgeExecutor)

type ForgeResult struct {
	*aicommon.Action
	Formated any
	Forge    *ForgeBlueprint
}

func (r *ForgeResult) GetForgeResult() *aicommon.ForgeResult {
	return convertForgeResultIntoCommonForgeResult(r)
}

func RegisterYakAiForge(cfg *YakForgeBlueprintConfig) error {
	blueprint, err := cfg.Build()
	if err != nil {
		return err
	}
	return RegisterYakForgeExecutor(cfg.Name, func(ctx context.Context, items []*ypb.ExecParamItem, opts ...aicommon.ConfigOption) (*ForgeResult, error) {
		ins, err := blueprint.CreateCoordinator(ctx, items, opts...)
		if err != nil {
			return nil, err
		}
		err = ins.Run()
		return ins.Result(), err
	})
}

func RegisterLiteForge(i string, params ...liteforgeapp.LiteForgeOption) error {
	lf, err := liteforgeapp.NewLiteForge(i, params...)
	if err != nil {
		return utils.Errorf("build lite forge failed: %v", err)
	}
	return RegisterForgeExecutor(i, func(ctx context.Context, params []*ypb.ExecParamItem, opts ...aicommon.ConfigOption) (*ForgeResult, error) {
		result, err := lf.Execute(ctx, params, opts...)
		if err != nil {
			return nil, err
		}
		return &ForgeResult{Action: result.Action}, nil
	})
}

func RegisterAIDBuildInForge(i string, params ...liteforgeapp.LiteForgeOption) error {
	lf, err := liteforgeapp.NewLiteForge(i, params...)
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

func RegisterYakForgeExecutor(i string, f ForgeExecutor) error {
	forgeMutex.Lock()
	if _, ok := yakForges[i]; ok {
		forgeMutex.Unlock()
		return utils.Errorf("forge %s already registered", i)
	}
	yakForges[i] = f
	forgeMutex.Unlock()
	return nil
}

var forgeNotFoundError = utils.Errorf("forge not found")

func ExecuteForge(
	forgeName string,
	ctx context.Context,
	params []*ypb.ExecParamItem,
	opts ...aicommon.ConfigOption,
) (*ForgeResult, error) {
	// 只在查找 forge 时持有读锁，找到后立即释放
	// 这样可以避免在 forge 执行期间（可能很长时间）阻塞其他 forge 的注册
	forgeMutex.RLock()
	forge, ok := forges[forgeName]
	if !ok {
		forge, ok = yakForges[forgeName]
	}
	forgeMutex.RUnlock()

	if ok {
		return forge(ctx, params, opts...)
	} else {
		return nil, utils.Wrapf(forgeNotFoundError, "forge %s not found", forgeName)
	}
}

func ExecuteForgeAndAutoRegister(forgeName string, ctx context.Context, params []*ypb.ExecParamItem, opts ...aicommon.ConfigOption) (*ForgeResult, error) {
	forgeIns, err := yakit.GetAIForgeByNameAndTypes(consts.GetGormProfileDatabase(), forgeName, schema.RunnableForgeTypes()...)
	if err == nil {
		cfg := NewYakForgeBlueprintConfigFromSchemaForge(forgeIns)
		blueprint, buildErr := cfg.Build()
		if buildErr != nil {
			return nil, utils.Wrap(buildErr, "failed to build forge")
		}
		ins, buildErr := blueprint.CreateCoordinator(ctx, params, opts...)
		if buildErr != nil {
			return nil, buildErr
		}
		buildErr = ins.Run()
		return ins.Result(), buildErr
	}

	forgeRes, execErr := ExecuteForge(forgeName, ctx, params, opts...)
	if execErr == nil {
		return forgeRes, nil
	}

	return forgeRes, utils.Wrap(execErr, "failed to execute forge")
}

func convertForgeResultIntoCommonForgeResult(fr *ForgeResult) *aicommon.ForgeResult {
	if fr == nil {
		return nil
	}
	name := ""
	if fr.Forge != nil {
		name = fr.Forge.Name
	}
	return &aicommon.ForgeResult{Action: fr.Action, Name: name, Formated: fr.Formated}
}

func init() {
	aicommon.RegisterPresetForgeExecuteCallback(func(name string, ctx context.Context, params any, opts ...aicommon.ConfigOption) (*aicommon.ForgeResult, error) {
		var finalParams []*ypb.ExecParamItem
		switch paramIns := params.(type) {
		case []*ypb.ExecParamItem:
			finalParams = paramIns
		case aitool.InvokeParams:
			for k, v := range paramIns {
				finalParams = append(finalParams, &ypb.ExecParamItem{Key: k, Value: utils.InterfaceToString(v)})
			}
		default:
			finalParams = []*ypb.ExecParamItem{
				{Key: "query", Value: utils.InterfaceToString(params)},
			}
		}
		result, err := ExecuteForgeAndAutoRegister(name, ctx, finalParams, opts...)
		common := convertForgeResultIntoCommonForgeResult(result)
		if common != nil && common.Name == "" {
			common.Name = name
		}
		return common, err
	})
}

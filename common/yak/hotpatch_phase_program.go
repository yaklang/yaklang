package yak

import (
	"context"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type hotPatchPhaseProgram struct {
	engine   *antlr4yak.Engine
	hookLock sync.Mutex
}

func compileHotPatchPhaseProgram(ctx context.Context, raw string, caller YakitCallerIf, params ...*ypb.ExecParamItem) (*hotPatchPhaseProgram, HotPatchRuntimeMode, error) {
	mode, err := DetectHotPatchRuntimeMode(ctx, raw)
	if err != nil {
		return nil, HotPatchRuntimeModeNone, err
	}
	if mode != HotPatchRuntimeModePhase {
		return nil, mode, nil
	}

	scriptEngine := NewScriptEngine(2)
	yakitContext := CreateYakitPluginContext("").WithContext(ctx)
	if caller != nil {
		client := yaklib.NewVirtualYakitClient(caller)
		db := consts.GetGormProjectDatabase()
		scriptEngine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
			engine.OverrideRuntimeGlobalVariables(map[string]any{
				"yakit_output": FeedbackFactory(db, caller, false, "default"),
				"yakit_save":   FeedbackFactory(db, caller, true, "default"),
				"yakit_status": func(id string, i interface{}) {
					FeedbackFactory(db, caller, false, id)(&yaklib.YakitStatusCard{
						Id:   id,
						Data: fmt.Sprint(i),
					})
				},
				"yakit": yaklib.GetExtYakitLibByClient(client),
			})
			return nil
		})
		yakitContext = yakitContext.WithYakitClient(client)
	}
	if len(params) > 0 {
		args := make([]string, 0, len(params)*2)
		for _, param := range params {
			args = append(args, "--"+param.GetKey(), fmt.Sprintf("%s", param.GetValue()))
		}
		yakitContext.WithCliApp(GetHookCliApp(args))
	}
	scriptEngine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		BindYakitPluginContextToEngine(engine, yakitContext)
		return nil
	})

	engine, err := scriptEngine.ExecuteEx(raw, map[string]any{})
	if err != nil {
		return nil, HotPatchRuntimeModeNone, err
	}
	return &hotPatchPhaseProgram{engine: engine}, mode, nil
}

func (p *hotPatchPhaseProgram) CallPhase(ctx context.Context, phase string, hotCtx *HotPatchPhaseContext) {
	if p == nil || p.engine == nil || hotCtx == nil {
		return
	}
	p.hookLock.Lock()
	defer p.hookLock.Unlock()

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("hotpatch phase %s panic: %v", phase, err)
		}
	}()

	raw, ok := p.engine.GetVar(phase)
	if !ok {
		return
	}
	if _, ok := raw.(*yakvm.Function); !ok {
		return
	}
	if _, err := p.engine.CallYakFunction(ctx, phase, []any{hotCtx}); err != nil {
		log.Errorf("call hotpatch phase %s failed: %v", phase, err)
	}
}

func extractHotPatchURL(raw []byte, isHTTPS bool) string {
	if len(raw) == 0 {
		return ""
	}
	if u, _ := lowhttp.ExtractURLFromHTTPRequestRaw(raw, isHTTPS); u != nil {
		return u.String()
	}
	return ""
}

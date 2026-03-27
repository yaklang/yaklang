package yak

import (
	"context"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type hotPatchPhaseWebFuzzerRuntimeFactory struct {
	runtimeCtx    context.Context
	globalProgram *hotPatchPhaseProgram
	moduleProgram *hotPatchPhaseProgram
}

type hotPatchPhaseWebFuzzerRuntime struct {
	factory       *hotPatchPhaseWebFuzzerRuntimeFactory
	isHTTPS       bool
	source        string
	originRequest []byte
	phaseCtx      *HotPatchPhaseContext
}

func BuildHotPatchHTTPPoolRuntimeFactoryChained(
	ctx context.Context,
	chain HotPatchChain,
	caller YakitCallerIf,
	params ...*ypb.ExecParamItem,
) (mutate.HTTPPoolRequestHookRuntimeFactory, HotPatchRuntimeMode, error) {
	mode, err := resolveChainedHotPatchMode(ctx, chain)
	if err != nil {
		return nil, HotPatchRuntimeModeNone, err
	}
	if mode != HotPatchRuntimeModePhase {
		return nil, mode, nil
	}

	globalProgram, _, err := compileHotPatchPhaseProgram(ctx, chain.GlobalCode, caller, params...)
	if err != nil {
		return nil, HotPatchRuntimeModeNone, err
	}
	moduleProgram, _, err := compileHotPatchPhaseProgram(ctx, chain.ModuleCode, caller, params...)
	if err != nil {
		return nil, HotPatchRuntimeModeNone, err
	}

	factory := &hotPatchPhaseWebFuzzerRuntimeFactory{
		runtimeCtx:    ctx,
		globalProgram: globalProgram,
		moduleProgram: moduleProgram,
	}
	return factory.newRuntime, mode, nil
}

func (f *hotPatchPhaseWebFuzzerRuntimeFactory) newRuntime(meta *mutate.HTTPPoolRequestHookRuntimeMeta) mutate.HTTPPoolRequestHookRuntime {
	if f == nil || meta == nil {
		return nil
	}
	return &hotPatchPhaseWebFuzzerRuntime{
		factory:       f,
		isHTTPS:       meta.IsHTTPS,
		source:        "webfuzzer",
		originRequest: cloneHotPatchBytes(meta.OriginRequest),
	}
}

func (r *hotPatchPhaseWebFuzzerRuntime) BeforeRequest(req []byte) []byte {
	phaseCtx := r.ensurePhaseContext(req)
	runPhaseProgram(r.factory.globalProgram, r.factory.runtimeCtx, HOOK_RequestIngress, phaseCtx)
	runPhaseProgram(r.factory.moduleProgram, r.factory.runtimeCtx, HOOK_RequestIngress, phaseCtx)
	runPhaseProgram(r.factory.globalProgram, r.factory.runtimeCtx, HOOK_RequestProcess, phaseCtx)
	runPhaseProgram(r.factory.moduleProgram, r.factory.runtimeCtx, HOOK_RequestProcess, phaseCtx)
	runPhaseProgram(r.factory.moduleProgram, r.factory.runtimeCtx, HOOK_RequestEgress, phaseCtx)
	runPhaseProgram(r.factory.globalProgram, r.factory.runtimeCtx, HOOK_RequestEgress, phaseCtx)
	if phaseCtx.Dropped {
		log.Errorf("Drop action is not supported in webfuzzer request phase runtime")
	}
	if len(phaseCtx.Request) == 0 {
		return req
	}
	return cloneHotPatchBytes(phaseCtx.Request)
}

func (r *hotPatchPhaseWebFuzzerRuntime) AfterRequest(req []byte, rsp []byte) []byte {
	phaseCtx := r.ensurePhaseContext(req)
	phaseCtx.SetRequest(req)
	if len(phaseCtx.OriginResponse) == 0 {
		phaseCtx.SetOriginResponse(rsp)
	}
	phaseCtx.SetResponse(rsp)
	runPhaseProgram(r.factory.globalProgram, r.factory.runtimeCtx, HOOK_ResponseIngress, phaseCtx)
	runPhaseProgram(r.factory.moduleProgram, r.factory.runtimeCtx, HOOK_ResponseIngress, phaseCtx)
	runPhaseProgram(r.factory.globalProgram, r.factory.runtimeCtx, HOOK_ResponseProcess, phaseCtx)
	runPhaseProgram(r.factory.moduleProgram, r.factory.runtimeCtx, HOOK_ResponseProcess, phaseCtx)
	runPhaseProgram(r.factory.moduleProgram, r.factory.runtimeCtx, HOOK_ResponseEgress, phaseCtx)
	runPhaseProgram(r.factory.globalProgram, r.factory.runtimeCtx, HOOK_ResponseEgress, phaseCtx)
	if phaseCtx.Dropped {
		log.Errorf("Drop action is not supported in webfuzzer response phase runtime")
		return rsp
	}
	if len(phaseCtx.ClientResponse) > 0 {
		return cloneHotPatchBytes(phaseCtx.ClientResponse)
	}
	if len(phaseCtx.Response) == 0 {
		return rsp
	}
	return cloneHotPatchBytes(phaseCtx.Response)
}

func (r *hotPatchPhaseWebFuzzerRuntime) MirrorHTTPFlow(_ []byte, _ []byte, _ map[string]string) map[string]string {
	return nil
}

func (r *hotPatchPhaseWebFuzzerRuntime) RetryHandler(_ int, _ []byte, _ []byte, _ func(...[]byte)) {
}

func (r *hotPatchPhaseWebFuzzerRuntime) CustomFailureChecker(_ []byte, _ []byte, _ func(string)) {
}

func (r *hotPatchPhaseWebFuzzerRuntime) MockHTTPRequest(_ string, _ []byte, mockResponse func(interface{})) {
	if r == nil || r.phaseCtx == nil || len(r.phaseCtx.ClientResponse) == 0 || mockResponse == nil {
		return
	}
	mockResponse(cloneHotPatchBytes(r.phaseCtx.ClientResponse))
}

func (r *hotPatchPhaseWebFuzzerRuntime) ensurePhaseContext(req []byte) *HotPatchPhaseContext {
	if r.phaseCtx != nil {
		r.phaseCtx.IsHTTPS = r.isHTTPS
		r.phaseCtx.Source = r.source
		r.phaseCtx.SetRequest(req)
		return r.phaseCtx
	}
	r.phaseCtx = NewHotPatchRequestPhaseContext(
		r.source,
		r.isHTTPS,
		extractHotPatchURL(req, r.isHTTPS),
		r.originRequest,
		req,
		nil,
		nil,
	)
	return r.phaseCtx
}

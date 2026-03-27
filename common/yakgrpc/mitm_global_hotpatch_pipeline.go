package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type mitmGlobalHotPatchPipeline struct {
	module *yak.MixPluginCaller
	ctx    context.Context

	buildGlobalCaller func() (*yak.MixPluginCaller, error)
	globalCaller      atomic.Value // *yak.MixPluginCaller

	loadedVersion int64
	mu            sync.Mutex
}

func newMitmGlobalHotPatchPipeline(
	ctx context.Context,
	module *yak.MixPluginCaller,
	initialGlobal *yak.MixPluginCaller,
	buildGlobal func() (*yak.MixPluginCaller, error),
) *mitmGlobalHotPatchPipeline {
	if ctx == nil {
		ctx = context.Background()
	}
	p := &mitmGlobalHotPatchPipeline{
		module:            module,
		ctx:               ctx,
		buildGlobalCaller: buildGlobal,
		loadedVersion:     -1,
	}
	p.globalCaller.Store(initialGlobal)
	return p
}

func (p *mitmGlobalHotPatchPipeline) ensureGlobalHotPatchLoaded() {
	enabled, version, code := yakit.GetGlobalHotPatchVersionAndCode()
	if atomic.LoadInt64(&p.loadedVersion) == version {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	enabled, version, code = yakit.GetGlobalHotPatchVersionAndCode()
	if atomic.LoadInt64(&p.loadedVersion) == version {
		return
	}

	caller, err := p.buildGlobalCaller()
	if err != nil {
		log.Errorf("build global hotpatch caller failed: %v", err)
		return
	}
	if enabled {
		if err := caller.LoadHotPatchSilently(p.ctx, nil, code); err != nil {
			log.Errorf("load global hotpatch failed(version=%d enabled=%v): %v", version, enabled, err)
			return
		}
	}

	p.globalCaller.Store(caller)
	atomic.StoreInt64(&p.loadedVersion, version)
}

func (p *mitmGlobalHotPatchPipeline) getGlobalCaller() *yak.MixPluginCaller {
	v := p.globalCaller.Load()
	if v == nil {
		return nil
	}
	caller, _ := v.(*yak.MixPluginCaller)
	return caller
}

func (p *mitmGlobalHotPatchPipeline) CallBeforeRequestWithCtx(
	runtimeCtx context.Context,
	isHttps bool,
	u string,
	originReq []byte,
	req []byte,
) *yak.HotPatchPhaseContext {
	return p.CallBeforeRequestWithReq(runtimeCtx, nil, isHttps, u, originReq, req)
}

func (p *mitmGlobalHotPatchPipeline) CallBeforeRequestWithReq(
	runtimeCtx context.Context,
	carrier *http.Request,
	isHttps bool,
	u string,
	originReq []byte,
	req []byte,
) *yak.HotPatchPhaseContext {
	p.ensureGlobalHotPatchLoaded()

	global := p.getGlobalCaller()
	mode, err := resolveHotPatchChainMode(hotPatchModeOf(global), hotPatchModeOf(p.module))
	if err != nil {
		log.Errorf("resolve request hotpatch runtime failed: %v", err)
		return legacyBeforeRequest(global, p.module, runtimeCtx, isHttps, u, originReq, req)
	}
	if mode != yak.HotPatchRuntimeModePhase {
		return legacyBeforeRequest(global, p.module, runtimeCtx, isHttps, u, originReq, req)
	}

	phaseCtx := prepareMitmRequestPhaseContext(carrier, "mitm", isHttps, u, originReq, req)
	runRequestPhase(global, p.module, runtimeCtx, phaseCtx)
	if shouldContinueLegacyRequestHooks(phaseCtx) {
		if out := p.module.CallBeforeRequestWithCtx(runtimeCtx, isHttps, u, originReq, phaseCtx.Request); len(out) > 0 {
			phaseCtx.Request = out
		}
	}
	if !hasRequestOutcome(req, phaseCtx) {
		return nil
	}
	return phaseCtx
}

func (p *mitmGlobalHotPatchPipeline) CallAfterRequestWithCtx(
	runtimeCtx context.Context,
	isHttps bool,
	u string,
	originReq []byte,
	req []byte,
	originRsp []byte,
	rsp []byte,
) *yak.HotPatchPhaseContext {
	return p.CallAfterRequestWithReq(runtimeCtx, nil, isHttps, u, originReq, req, originRsp, rsp)
}

func (p *mitmGlobalHotPatchPipeline) CallAfterRequestWithReq(
	runtimeCtx context.Context,
	carrier *http.Request,
	isHttps bool,
	u string,
	originReq []byte,
	req []byte,
	originRsp []byte,
	rsp []byte,
) *yak.HotPatchPhaseContext {
	p.ensureGlobalHotPatchLoaded()

	global := p.getGlobalCaller()
	mode, err := resolveHotPatchChainMode(hotPatchModeOf(global), hotPatchModeOf(p.module))
	if err != nil {
		log.Errorf("resolve response hotpatch runtime failed: %v", err)
		return legacyAfterRequest(global, p.module, runtimeCtx, isHttps, u, originReq, req, originRsp, rsp)
	}
	if mode != yak.HotPatchRuntimeModePhase {
		return legacyAfterRequest(global, p.module, runtimeCtx, isHttps, u, originReq, req, originRsp, rsp)
	}

	phaseCtx := prepareMitmResponsePhaseContext(carrier, "mitm", isHttps, u, originReq, req, originRsp, rsp)
	runResponsePhase(global, p.module, runtimeCtx, phaseCtx)
	if shouldContinueLegacyResponseHooks(phaseCtx) {
		if out := p.module.CallAfterRequestWithCtx(runtimeCtx, isHttps, u, originReq, req, originRsp, phaseCtx.Response); len(out) > 0 {
			phaseCtx.Response = out
		}
	}
	if !hasResponseOutcome(rsp, phaseCtx) {
		return nil
	}
	return phaseCtx
}

func (p *mitmGlobalHotPatchPipeline) CallHijackResponseExWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string,
	getRequest, getResponse, reject, drop func() interface{},
) {
	p.ensureGlobalHotPatchLoaded()
	if global := p.getGlobalCaller(); global != nil {
		global.CallHijackResponseExWithCtx(runtimeCtx, isHttps, u, getRequest, getResponse, reject, drop)
	}
	if p.module != nil {
		p.module.CallHijackResponseExWithCtx(runtimeCtx, isHttps, u, getRequest, getResponse, reject, drop)
	}
}

func (p *mitmGlobalHotPatchPipeline) CallHijackResponseWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string,
	getResponse, reject, drop func() interface{},
) {
	p.ensureGlobalHotPatchLoaded()
	if global := p.getGlobalCaller(); global != nil {
		global.CallHijackResponseWithCtx(runtimeCtx, isHttps, u, getResponse, reject, drop)
	}
	if p.module != nil {
		p.module.CallHijackResponseWithCtx(runtimeCtx, isHttps, u, getResponse, reject, drop)
	}
}

func (p *mitmGlobalHotPatchPipeline) CallMockHTTPRequestWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string,
	getRequest func() interface{},
	mockResponse func(rsp interface{}),
) {
	p.ensureGlobalHotPatchLoaded()
	if global := p.getGlobalCaller(); global != nil {
		global.CallMockHTTPRequestWithCtx(runtimeCtx, isHttps, u, getRequest, mockResponse)
	}
	if p.module != nil {
		p.module.CallMockHTTPRequestWithCtx(runtimeCtx, isHttps, u, getRequest, mockResponse)
	}
}

func (p *mitmGlobalHotPatchPipeline) CallHijackRequestWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string,
	getRequest, reject, drop func() interface{},
) {
	p.ensureGlobalHotPatchLoaded()
	if global := p.getGlobalCaller(); global != nil {
		global.CallHijackRequestWithCtx(runtimeCtx, isHttps, u, getRequest, reject, drop)
	}
	if p.module != nil {
		p.module.CallHijackRequestWithCtx(runtimeCtx, isHttps, u, getRequest, reject, drop)
	}
}

func (p *mitmGlobalHotPatchPipeline) MirrorHTTPFlowWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string,
	req, rsp, body []byte,
	filters ...bool,
) {
	p.ensureGlobalHotPatchLoaded()
	if global := p.getGlobalCaller(); global != nil {
		global.MirrorHTTPFlowWithCtx(runtimeCtx, isHttps, u, req, rsp, body, filters...)
	}
	if p.module != nil {
		p.module.MirrorHTTPFlowWithCtx(runtimeCtx, isHttps, u, req, rsp, body, filters...)
	}
}

func (p *mitmGlobalHotPatchPipeline) HijackSaveHTTPFlowEx(
	runtimeCtx context.Context,
	flow *schema.HTTPFlow,
	callback func(),
	reject func(httpFlow *schema.HTTPFlow),
	drop func(),
) {
	p.HijackSaveHTTPFlowWithReqEx(runtimeCtx, nil, flow, callback, reject, drop)
}

func (p *mitmGlobalHotPatchPipeline) HijackSaveHTTPFlowWithReqEx(
	runtimeCtx context.Context,
	carrier *http.Request,
	flow *schema.HTTPFlow,
	callback func(),
	reject func(httpFlow *schema.HTTPFlow),
	drop func(),
) {
	defer clearMitmHotPatchPhaseContext(carrier)

	p.ensureGlobalHotPatchLoaded()

	global := p.getGlobalCaller()
	mode, err := resolveHotPatchChainMode(hotPatchModeOf(global), hotPatchModeOf(p.module))
	if err != nil {
		log.Errorf("resolve flowArchive hotpatch runtime failed: %v", err)
		legacyArchiveFlow(global, p.module, runtimeCtx, flow, callback, reject, drop)
		return
	}
	if mode != yak.HotPatchRuntimeModePhase {
		legacyArchiveFlow(global, p.module, runtimeCtx, flow, callback, reject, drop)
		return
	}

	phaseCtx := prepareMitmArchivePhaseContext(carrier, "mitm", flow)
	if p.module != nil {
		p.module.CallHotPatchPhaseWithCtx(runtimeCtx, yak.HOOK_FlowArchive, phaseCtx)
	}
	if global != nil {
		global.CallHotPatchPhaseWithCtx(runtimeCtx, yak.HOOK_FlowArchive, phaseCtx)
	}
	phaseCtx.ApplyArchiveResultToFlow()
	if phaseCtx.ArchiveSkipped && drop != nil {
		drop()
	}
	if callback != nil {
		callback()
	}
}

func legacyBeforeRequest(global, module *yak.MixPluginCaller, runtimeCtx context.Context, isHttps bool, u string, originReq, req []byte) *yak.HotPatchPhaseContext {
	cur := req
	if global != nil {
		if out := global.CallBeforeRequestWithCtx(runtimeCtx, isHttps, u, originReq, cur); len(out) > 0 {
			cur = out
		}
	}
	if module != nil {
		if out := module.CallBeforeRequestWithCtx(runtimeCtx, isHttps, u, originReq, cur); len(out) > 0 {
			cur = out
		}
	}
	if bytes.Equal(cur, req) {
		return nil
	}
	ctx := yak.NewHotPatchRequestPhaseContext("mitm", isHttps, u, originReq, cur, nil, nil)
	ctx.Request = cur
	return ctx
}

func legacyAfterRequest(global, module *yak.MixPluginCaller, runtimeCtx context.Context, isHttps bool, u string, originReq, req, originRsp, rsp []byte) *yak.HotPatchPhaseContext {
	cur := rsp
	if global != nil {
		if out := global.CallAfterRequestWithCtx(runtimeCtx, isHttps, u, originReq, req, originRsp, cur); len(out) > 0 {
			cur = out
		}
	}
	if module != nil {
		if out := module.CallAfterRequestWithCtx(runtimeCtx, isHttps, u, originReq, req, originRsp, cur); len(out) > 0 {
			cur = out
		}
	}
	if bytes.Equal(cur, rsp) {
		return nil
	}
	ctx := yak.NewHotPatchRequestPhaseContext("mitm", isHttps, u, originReq, req, originRsp, cur)
	ctx.Response = cur
	return ctx
}

func legacyArchiveFlow(global, module *yak.MixPluginCaller, runtimeCtx context.Context, flow *schema.HTTPFlow, callback func(), reject func(httpFlow *schema.HTTPFlow), drop func()) {
	if callback == nil {
		if global != nil {
			global.HijackSaveHTTPFlowEx(runtimeCtx, flow, nil, reject, drop)
		}
		if module != nil {
			module.HijackSaveHTTPFlowEx(runtimeCtx, flow, nil, reject, drop)
		}
		return
	}

	var dropped atomic.Bool
	wrappedDrop := func() {
		dropped.Store(true)
		if drop != nil {
			drop()
		}
	}

	callModule := func() {
		if dropped.Load() {
			callback()
			return
		}
		if module != nil {
			module.HijackSaveHTTPFlowEx(runtimeCtx, flow, callback, reject, wrappedDrop)
			return
		}
		callback()
	}

	if global != nil {
		global.HijackSaveHTTPFlowEx(runtimeCtx, flow, callModule, reject, wrappedDrop)
		return
	}
	callModule()
}

func hotPatchModeOf(caller *yak.MixPluginCaller) yak.HotPatchRuntimeMode {
	if caller == nil {
		return yak.HotPatchRuntimeModeNone
	}
	return caller.HotPatchMode()
}

func resolveHotPatchChainMode(global, module yak.HotPatchRuntimeMode) (yak.HotPatchRuntimeMode, error) {
	if global == yak.HotPatchRuntimeModeNone && module == yak.HotPatchRuntimeModeNone {
		return yak.HotPatchRuntimeModeNone, nil
	}
	if isPhaseCompatible(global) && isPhaseCompatible(module) {
		if global == yak.HotPatchRuntimeModePhase || module == yak.HotPatchRuntimeModePhase {
			return yak.HotPatchRuntimeModePhase, nil
		}
	}
	if isLegacyCompatible(global) && isLegacyCompatible(module) {
		if global == yak.HotPatchRuntimeModeLegacy || module == yak.HotPatchRuntimeModeLegacy {
			return yak.HotPatchRuntimeModeLegacy, nil
		}
	}
	return yak.HotPatchRuntimeModeNone, fmt.Errorf("global hotpatch mode=%q conflicts with module hotpatch mode=%q", global, module)
}

func isPhaseCompatible(mode yak.HotPatchRuntimeMode) bool {
	return mode == yak.HotPatchRuntimeModeNone || mode == yak.HotPatchRuntimeModePhase
}

func isLegacyCompatible(mode yak.HotPatchRuntimeMode) bool {
	return mode == yak.HotPatchRuntimeModeNone || mode == yak.HotPatchRuntimeModeLegacy
}

func runRequestPhase(global, module *yak.MixPluginCaller, runtimeCtx context.Context, ctx *yak.HotPatchPhaseContext) {
	runPhase(global, runtimeCtx, yak.HOOK_RequestIngress, ctx)
	runPhase(module, runtimeCtx, yak.HOOK_RequestIngress, ctx)
	runPhase(global, runtimeCtx, yak.HOOK_RequestProcess, ctx)
	runPhase(module, runtimeCtx, yak.HOOK_RequestProcess, ctx)
	runPhase(module, runtimeCtx, yak.HOOK_RequestEgress, ctx)
	runPhase(global, runtimeCtx, yak.HOOK_RequestEgress, ctx)
}

func runResponsePhase(global, module *yak.MixPluginCaller, runtimeCtx context.Context, ctx *yak.HotPatchPhaseContext) {
	runPhase(global, runtimeCtx, yak.HOOK_ResponseIngress, ctx)
	runPhase(module, runtimeCtx, yak.HOOK_ResponseIngress, ctx)
	runPhase(global, runtimeCtx, yak.HOOK_ResponseProcess, ctx)
	runPhase(module, runtimeCtx, yak.HOOK_ResponseProcess, ctx)
	runPhase(module, runtimeCtx, yak.HOOK_ResponseEgress, ctx)
	runPhase(global, runtimeCtx, yak.HOOK_ResponseEgress, ctx)
}

func runPhase(caller *yak.MixPluginCaller, runtimeCtx context.Context, phase string, ctx *yak.HotPatchPhaseContext) {
	if caller == nil || ctx == nil || ctx.Stopped || ctx.Dropped {
		return
	}
	caller.CallHotPatchPhaseWithCtx(runtimeCtx, phase, ctx)
	if isMitmHotPatchRequestPhase(phase) {
		ctx.RefreshRequestMetadata()
	}
}

func isMitmHotPatchRequestPhase(phase string) bool {
	return phase == yak.HOOK_RequestIngress ||
		phase == yak.HOOK_RequestProcess ||
		phase == yak.HOOK_RequestEgress
}

func shouldContinueLegacyRequestHooks(ctx *yak.HotPatchPhaseContext) bool {
	if ctx == nil {
		return true
	}
	return !ctx.Dropped && len(ctx.ClientResponse) == 0
}

func shouldContinueLegacyResponseHooks(ctx *yak.HotPatchPhaseContext) bool {
	if ctx == nil {
		return true
	}
	return !ctx.Dropped && len(ctx.ClientResponse) == 0
}

func hasRequestOutcome(origin []byte, ctx *yak.HotPatchPhaseContext) bool {
	if ctx == nil {
		return false
	}
	return ctx.Dropped || len(ctx.ClientResponse) > 0 || !bytes.Equal(origin, ctx.Request)
}

func hasResponseOutcome(origin []byte, ctx *yak.HotPatchPhaseContext) bool {
	if ctx == nil {
		return false
	}
	if ctx.Dropped || len(ctx.ClientResponse) > 0 {
		return true
	}
	return !bytes.Equal(origin, ctx.Response)
}

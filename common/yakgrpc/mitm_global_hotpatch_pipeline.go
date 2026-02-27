package yakgrpc

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type mitmGlobalHotPatchPipeline struct {
	module *yak.MixPluginCaller

	buildGlobalCaller func() (*yak.MixPluginCaller, error)
	globalCaller      atomic.Value // *yak.MixPluginCaller

	loadedVersion int64
	mu            sync.Mutex
}

func newMitmGlobalHotPatchPipeline(
	module *yak.MixPluginCaller,
	initialGlobal *yak.MixPluginCaller,
	buildGlobal func() (*yak.MixPluginCaller, error),
) *mitmGlobalHotPatchPipeline {
	p := &mitmGlobalHotPatchPipeline{
		module:            module,
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
	loadCode := ""
	if enabled {
		loadCode = code
	}
	if err := caller.LoadHotPatch(utils.TimeoutContextSeconds(consts.GetGlobalCallerLoadPluginTimeout()), nil, loadCode); err != nil {
		log.Errorf("load global hotpatch failed(version=%d enabled=%v): %v", version, enabled, err)
		return
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

func (p *mitmGlobalHotPatchPipeline) CallBeforeRequestWithCtx(runtimeCtx context.Context, isHttps bool, u string, originReq, req []byte) []byte {
	p.ensureGlobalHotPatchLoaded()

	cur := req
	if global := p.getGlobalCaller(); global != nil {
		if out := global.CallBeforeRequestWithCtx(runtimeCtx, isHttps, u, originReq, cur); len(out) > 0 {
			cur = out
		}
	}
	if p.module != nil {
		if out := p.module.CallBeforeRequestWithCtx(runtimeCtx, isHttps, u, originReq, cur); len(out) > 0 {
			cur = out
		}
	}

	if bytes.Equal(cur, req) {
		return nil
	}
	return cur
}

func (p *mitmGlobalHotPatchPipeline) CallAfterRequestWithCtx(runtimeCtx context.Context, isHttps bool, u string, originReq, req, originRsp, rsp []byte) []byte {
	p.ensureGlobalHotPatchLoaded()

	cur := rsp
	if global := p.getGlobalCaller(); global != nil {
		if out := global.CallAfterRequestWithCtx(runtimeCtx, isHttps, u, originReq, req, originRsp, cur); len(out) > 0 {
			cur = out
		}
	}
	if p.module != nil {
		if out := p.module.CallAfterRequestWithCtx(runtimeCtx, isHttps, u, originReq, req, originRsp, cur); len(out) > 0 {
			cur = out
		}
	}

	if bytes.Equal(cur, rsp) {
		return nil
	}
	return cur
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
	p.ensureGlobalHotPatchLoaded()

	global := p.getGlobalCaller()
	module := p.module
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

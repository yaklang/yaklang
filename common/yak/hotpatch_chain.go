package yak

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/fuzztag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type HotPatchChain struct {
	GlobalCode string
	ModuleCode string
}

type (
	hookBeforeRequestFunc      func(https bool, originReq []byte, req []byte) []byte
	hookAfterRequestFunc       func(https bool, originReq []byte, req []byte, originRsp []byte, rsp []byte) []byte
	hookMirrorHTTPFlowFunc     func([]byte, []byte, map[string]string) map[string]string
	hookRetryHandlerFunc       func(bool, int, []byte, []byte, func(...[]byte))
	hookCustomFailureChecker   func(bool, []byte, []byte, func(string))
	hookMockHTTPRequestHandler func(bool, string, []byte, func(interface{}))
)

func MutateHookCallerChained(
	ctx context.Context,
	chain HotPatchChain,
	caller YakitCallerIf,
	params ...*ypb.ExecParamItem,
) (
	hookBeforeRequestFunc,
	hookAfterRequestFunc,
	hookMirrorHTTPFlowFunc,
	hookRetryHandlerFunc,
	hookCustomFailureChecker,
	hookMockHTTPRequestHandler,
) {
	mode, err := resolveChainedHotPatchMode(ctx, chain)
	if err != nil {
		log.Errorf("resolve hotpatch chain runtime failed: %v", err)
		return legacyMutateHookCallerChained(ctx, chain, caller, params...)
	}
	if mode == HotPatchRuntimeModePhase {
		return buildPhaseHookCallersChained(ctx, chain, caller, params...)
	}
	return legacyMutateHookCallerChained(ctx, chain, caller, params...)
}

func legacyMutateHookCallerChained(
	ctx context.Context,
	chain HotPatchChain,
	caller YakitCallerIf,
	params ...*ypb.ExecParamItem,
) (
	hookBeforeRequestFunc,
	hookAfterRequestFunc,
	hookMirrorHTTPFlowFunc,
	hookRetryHandlerFunc,
	hookCustomFailureChecker,
	hookMockHTTPRequestHandler,
) {
	gBefore, gAfter, gMirror, gRetry, gFail, gMock := MutateHookCaller(ctx, chain.GlobalCode, caller, params...)
	mBefore, mAfter, mMirror, mRetry, mFail, mMock := MutateHookCaller(ctx, chain.ModuleCode, caller, params...)
	return chainBeforeRequest(gBefore, mBefore),
		chainAfterRequest(gAfter, mAfter),
		chainMirrorHTTPFlow(gMirror, mMirror),
		chainRetryHandler(gRetry, mRetry),
		chainCustomFailureChecker(gFail, mFail),
		chainMockHTTPRequest(gMock, mMock)
}

func resolveChainedHotPatchMode(ctx context.Context, chain HotPatchChain) (HotPatchRuntimeMode, error) {
	globalMode, err := DetectHotPatchRuntimeMode(ctx, chain.GlobalCode)
	if err != nil {
		return HotPatchRuntimeModeNone, err
	}
	moduleMode, err := DetectHotPatchRuntimeMode(ctx, chain.ModuleCode)
	if err != nil {
		return HotPatchRuntimeModeNone, err
	}
	if globalMode == HotPatchRuntimeModeNone && moduleMode == HotPatchRuntimeModeNone {
		return HotPatchRuntimeModeNone, nil
	}
	if isPhaseChainMode(globalMode, moduleMode) {
		return HotPatchRuntimeModePhase, nil
	}
	if isLegacyChainMode(globalMode, moduleMode) {
		return HotPatchRuntimeModeLegacy, nil
	}
	return HotPatchRuntimeModeNone, utils.Errorf("global hotpatch mode=%q conflicts with module hotpatch mode=%q", globalMode, moduleMode)
}

func isPhaseChainMode(global, module HotPatchRuntimeMode) bool {
	if (global != HotPatchRuntimeModeNone && global != HotPatchRuntimeModePhase) ||
		(module != HotPatchRuntimeModeNone && module != HotPatchRuntimeModePhase) {
		return false
	}
	return global == HotPatchRuntimeModePhase || module == HotPatchRuntimeModePhase
}

func isLegacyChainMode(global, module HotPatchRuntimeMode) bool {
	if (global != HotPatchRuntimeModeNone && global != HotPatchRuntimeModeLegacy) ||
		(module != HotPatchRuntimeModeNone && module != HotPatchRuntimeModeLegacy) {
		return false
	}
	return global == HotPatchRuntimeModeLegacy || module == HotPatchRuntimeModeLegacy
}

type hotPatchPhaseMockStore struct {
	mockResponses sync.Map
}

func (s *hotPatchPhaseMockStore) store(req []byte, rsp []byte) {
	if len(req) == 0 || len(rsp) == 0 {
		return
	}
	s.mockResponses.Store(utils.CalcSha1(req), append([]byte(nil), rsp...))
}

func (s *hotPatchPhaseMockStore) take(req []byte) []byte {
	key := utils.CalcSha1(req)
	raw, ok := s.mockResponses.LoadAndDelete(key)
	if !ok {
		return nil
	}
	ret, _ := raw.([]byte)
	return ret
}

func buildPhaseHookCallersChained(
	ctx context.Context,
	chain HotPatchChain,
	caller YakitCallerIf,
	params ...*ypb.ExecParamItem,
) (
	hookBeforeRequestFunc,
	hookAfterRequestFunc,
	hookMirrorHTTPFlowFunc,
	hookRetryHandlerFunc,
	hookCustomFailureChecker,
	hookMockHTTPRequestHandler,
) {
	globalProgram, _, err := compileHotPatchPhaseProgram(ctx, chain.GlobalCode, caller, params...)
	if err != nil {
		log.Errorf("compile global hotpatch phase program failed: %v", err)
		return nil, nil, nil, nil, nil, nil
	}
	moduleProgram, _, err := compileHotPatchPhaseProgram(ctx, chain.ModuleCode, caller, params...)
	if err != nil {
		log.Errorf("compile module hotpatch phase program failed: %v", err)
		return nil, nil, nil, nil, nil, nil
	}

	mockStore := &hotPatchPhaseMockStore{}
	before := func(https bool, originReq []byte, req []byte) []byte {
		hotCtx := NewHotPatchRequestPhaseContext("webfuzzer", https, extractHotPatchURL(req, https), originReq, req, nil, nil)
		runPhaseProgram(globalProgram, ctx, HOOK_RequestIngress, hotCtx)
		runPhaseProgram(moduleProgram, ctx, HOOK_RequestIngress, hotCtx)
		runPhaseProgram(globalProgram, ctx, HOOK_RequestProcess, hotCtx)
		runPhaseProgram(moduleProgram, ctx, HOOK_RequestProcess, hotCtx)
		runPhaseProgram(moduleProgram, ctx, HOOK_RequestEgress, hotCtx)
		runPhaseProgram(globalProgram, ctx, HOOK_RequestEgress, hotCtx)
		if hotCtx.Dropped {
			log.Errorf("Drop action is not supported in webfuzzer request phase runtime")
		}
		if len(hotCtx.ClientResponse) > 0 {
			mockStore.store(hotCtx.Request, hotCtx.ClientResponse)
		}
		if len(hotCtx.Request) == 0 {
			return req
		}
		return hotCtx.Request
	}
	after := func(https bool, originReq []byte, req []byte, originRsp []byte, rsp []byte) []byte {
		hotCtx := NewHotPatchRequestPhaseContext("webfuzzer", https, extractHotPatchURL(req, https), originReq, req, originRsp, rsp)
		runPhaseProgram(globalProgram, ctx, HOOK_ResponseIngress, hotCtx)
		runPhaseProgram(moduleProgram, ctx, HOOK_ResponseIngress, hotCtx)
		runPhaseProgram(globalProgram, ctx, HOOK_ResponseProcess, hotCtx)
		runPhaseProgram(moduleProgram, ctx, HOOK_ResponseProcess, hotCtx)
		runPhaseProgram(moduleProgram, ctx, HOOK_ResponseEgress, hotCtx)
		runPhaseProgram(globalProgram, ctx, HOOK_ResponseEgress, hotCtx)
		if hotCtx.Dropped {
			log.Errorf("Drop action is not supported in webfuzzer response phase runtime")
			return rsp
		}
		if len(hotCtx.ClientResponse) > 0 {
			return hotCtx.ClientResponse
		}
		if len(hotCtx.Response) == 0 {
			return rsp
		}
		return hotCtx.Response
	}
	mock := func(_ bool, _ string, req []byte, mockResponse func(interface{})) {
		if rsp := mockStore.take(req); len(rsp) > 0 {
			mockResponse(rsp)
		}
	}
	return before, after, nil, nil, nil, mock
}

func runPhaseProgram(program *hotPatchPhaseProgram, ctx context.Context, phase string, hotCtx *HotPatchPhaseContext) {
	if program == nil || hotCtx == nil || hotCtx.Dropped || hotCtx.Stopped {
		return
	}
	program.CallPhase(ctx, phase, hotCtx)
	if isHotPatchRequestPhase(phase) {
		hotCtx.RefreshRequestMetadata()
	}
}

func isHotPatchRequestPhase(phase string) bool {
	return phase == HOOK_RequestIngress || phase == HOOK_RequestProcess || phase == HOOK_RequestEgress
}

func chainBeforeRequest(global, module hookBeforeRequestFunc) hookBeforeRequestFunc {
	if global == nil {
		return module
	}
	if module == nil {
		return global
	}
	return func(https bool, originReq []byte, req []byte) []byte {
		cur := req
		if out := global(https, originReq, cur); out != nil {
			cur = out
		}
		if out := module(https, originReq, cur); out != nil {
			cur = out
		}
		return cur
	}
}

func chainAfterRequest(global, module hookAfterRequestFunc) hookAfterRequestFunc {
	if global == nil {
		return module
	}
	if module == nil {
		return global
	}
	return func(https bool, originReq []byte, req []byte, originRsp []byte, rsp []byte) []byte {
		cur := rsp
		if out := global(https, originReq, req, originRsp, cur); out != nil {
			cur = out
		}
		if out := module(https, originReq, req, originRsp, cur); out != nil {
			cur = out
		}
		return cur
	}
}

func chainMirrorHTTPFlow(global, module hookMirrorHTTPFlowFunc) hookMirrorHTTPFlowFunc {
	if global == nil {
		return module
	}
	if module == nil {
		return global
	}
	return func(req []byte, rsp []byte, existed map[string]string) map[string]string {
		merged := copyStringMap(existed)
		out := make(map[string]string)

		if g := global(req, rsp, merged); g != nil {
			for k, v := range g {
				out[k] = v
				merged[k] = v
			}
		}
		if m := module(req, rsp, merged); m != nil {
			for k, v := range m {
				out[k] = v
				merged[k] = v
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
}

func chainRetryHandler(global, module hookRetryHandlerFunc) hookRetryHandlerFunc {
	if global == nil {
		return module
	}
	if module == nil {
		return global
	}
	return func(https bool, retryCount int, req []byte, rsp []byte, retryFunc func(...[]byte)) {
		global(https, retryCount, req, rsp, retryFunc)
		module(https, retryCount, req, rsp, retryFunc)
	}
}

func chainCustomFailureChecker(global, module hookCustomFailureChecker) hookCustomFailureChecker {
	if global == nil {
		return module
	}
	if module == nil {
		return global
	}
	return func(https bool, req []byte, rsp []byte, fail func(string)) {
		global(https, req, rsp, fail)
		module(https, req, rsp, fail)
	}
}

func chainMockHTTPRequest(global, module hookMockHTTPRequestHandler) hookMockHTTPRequestHandler {
	if global == nil {
		return module
	}
	if module == nil {
		return global
	}
	return func(https bool, url string, req []byte, mockResponse func(interface{})) {
		global(https, url, req, mockResponse)
		module(https, url, req, mockResponse)
	}
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return make(map[string]string)
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func Fuzz_WithAllHotPatchChained(ctx context.Context, chain HotPatchChain) []mutate.FuzzConfigOpt {
	handler := buildChainedHotPatchTagHandler(ctx, chain)
	if handler != nil {
		return []mutate.FuzzConfigOpt{
			mutate.Fuzz_WithExtraFuzzTag("yak", mutate.HotPatchFuzztag(handler)),
			mutate.Fuzz_WithExtraFuzzTag("yak:dyn", mutate.HotPatchDynFuzztag(handler)),
		}
	}
	return []mutate.FuzzConfigOpt{
		mutate.Fuzz_WithExtraFuzzTagHandler("yak", func(s string) []string { return []string{s} }),
		mutate.Fuzz_WithExtraFuzzTagHandler("yak:dyn", func(s string) []string { return []string{s} }),
	}
}

func buildChainedHotPatchTagHandler(ctx context.Context, chain HotPatchChain) func(string, func(string)) error {
	globalEnv := buildHotPatchTagEnv(ctx, chain.GlobalCode)
	moduleEnv := buildHotPatchTagEnv(ctx, chain.ModuleCode)
	if globalEnv == nil && moduleEnv == nil {
		return nil
	}

	return func(s string, yield func(string)) error {
		handle, _, _ := strings.Cut(s, "|")
		if hasHotPatchFunc(moduleEnv, handle) {
			return callHotPatchTag(ctx, moduleEnv, s, yield)
		}
		if hasHotPatchFunc(globalEnv, handle) {
			return callHotPatchTag(ctx, globalEnv, s, yield)
		}
		return hotPatchTagError(fmt.Sprintf("function %s not found", handle))
	}
}

func buildHotPatchTagEnv(ctx context.Context, code string) *antlr4yak.Engine {
	if strings.TrimSpace(code) == "" {
		return nil
	}
	engine := NewScriptEngine(1)
	codeEnv, err := engine.ExecuteExWithContext(ctx, code, make(map[string]interface{}))
	if err != nil {
		log.Errorf("load hotPatch code error: %s", err)
		return nil
	}
	return codeEnv
}

func hasHotPatchFunc(env *antlr4yak.Engine, handle string) bool {
	if env == nil {
		return false
	}
	v, ok := env.GetVar(handle)
	if !ok {
		return false
	}
	_, ok = v.(*yakvm.Function)
	return ok
}

func callHotPatchTag(ctx context.Context, env *antlr4yak.Engine, input string, yield func(string)) (err error) {
	handle, params, _ := strings.Cut(input, "|")

	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*yakvm.VMPanic); ok {
				log.Errorf("call hotPatch code error: %v", e.GetData())
				err = fmt.Errorf("%v", e.GetData())
			}
		}
	}()

	yakVar, ok := env.GetVar(handle)
	if !ok {
		return hotPatchTagError(fmt.Sprintf("function %s not found", handle))
	}
	yakFunc, ok := yakVar.(*yakvm.Function)
	if !ok {
		return hotPatchTagError(fmt.Sprintf("function %s not found", handle))
	}

	switch numIn := yakFunc.GetNumIn(); numIn {
	case 1:
		data, callErr := env.CallYakFunction(ctx, handle, []any{params})
		if callErr != nil {
			return hotPatchTagError(callErr.Error())
		}
		if data == nil {
			return hotPatchTagError("return nil")
		}
		for _, item := range utils.InterfaceToStringSlice(data) {
			yield(item)
		}
		return nil
	case 2:
		_, callErr := env.CallYakFunction(ctx, handle, []any{params, yield})
		if callErr != nil {
			return hotPatchTagError(callErr.Error())
		}
		return nil
	default:
		return hotPatchTagError("invalid function params")
	}
}

func hotPatchTagError(errStr string) error {
	errInfo := fmt.Sprintf("%s%s", fuzztag.YakHotPatchErr, errStr)
	log.Errorf("call hotPatch code error: %v", errStr)
	return utils.Error(errInfo)
}

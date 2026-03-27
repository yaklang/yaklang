package yak

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	HOOK_RequestIngress  = "requestIngress"
	HOOK_RequestProcess  = "requestProcess"
	HOOK_RequestEgress   = "requestEgress"
	HOOK_ResponseIngress = "responseIngress"
	HOOK_ResponseProcess = "responseProcess"
	HOOK_ResponseEgress  = "responseEgress"
	HOOK_FlowArchive     = "flowArchive"
)

type HotPatchRuntimeMode string

const (
	HotPatchRuntimeModeNone   HotPatchRuntimeMode = ""
	HotPatchRuntimeModeLegacy HotPatchRuntimeMode = "legacy"
	HotPatchRuntimeModePhase  HotPatchRuntimeMode = "phase"
)

var hotPatchPhaseHooks = []string{
	HOOK_RequestIngress,
	HOOK_RequestProcess,
	HOOK_RequestEgress,
	HOOK_ResponseIngress,
	HOOK_ResponseProcess,
	HOOK_ResponseEgress,
	HOOK_FlowArchive,
}

var legacyHotPatchHooks = []string{
	HOOK_MirrorFilteredHTTPFlow,
	HOOK_MirrorHTTPFlow,
	HOOK_MirrorNewWebsite,
	HOOK_MirrorNewWebsitePath,
	HOOK_MirrorNewWebsitePathParams,
	HOOK_HijackHTTPRequest,
	HOOK_HijackHTTPResponse,
	HOOK_HijackHTTPResponseEx,
	HOOK_MockHTTPRequest,
	HOOK_hijackSaveHTTPFlow,
	HOOK_BeforeRequest,
	HOOK_AfterRequest,
	HOOK_Analyze_HTTPFlow,
	HOOK_OnAnalyzeHTTPFlowFinish,
}

func DetectHotPatchRuntimeMode(ctx context.Context, code string) (HotPatchRuntimeMode, error) {
	_ = ctx
	if strings.TrimSpace(code) == "" {
		return HotPatchRuntimeModeNone, nil
	}

	usage, err := detectHotPatchHookUsage(code)
	if err != nil {
		return HotPatchRuntimeModeNone, err
	}

	if usage.legacy && usage.phase {
		return HotPatchRuntimeModeNone, utils.Error("mixed legacy hotpatch hooks and phase hooks are not allowed in one script")
	}
	if usage.phase {
		return HotPatchRuntimeModePhase, nil
	}
	if usage.legacy {
		return HotPatchRuntimeModeLegacy, nil
	}
	return HotPatchRuntimeModeNone, nil
}

func DefaultHotPatchPhaseCallArgumentHook(_ string, numIn int, args []any) []any {
	if numIn <= 0 {
		return nil
	}
	if len(args) == 0 {
		return args
	}
	return args[:1]
}

func (m *MixPluginCaller) setHotPatchMode(mode HotPatchRuntimeMode) {
	m.hotPatchMu.Lock()
	defer m.hotPatchMu.Unlock()
	m.hotPatchMode = mode
}

func (m *MixPluginCaller) HotPatchMode() HotPatchRuntimeMode {
	m.hotPatchMu.RLock()
	defer m.hotPatchMu.RUnlock()
	return m.hotPatchMode
}

func (m *MixPluginCaller) CallHotPatchPhaseWithCtx(runtimeCtx context.Context, phase string, hotCtx *HotPatchPhaseContext) {
	if m == nil || hotCtx == nil {
		return
	}
	if hotCtx.URL != "" && !m.IsPassed(hotCtx.URL) {
		log.Infof("call hotpatch phase error: url[%v] not passed", hotCtx.URL)
		return
	}
	if !m.callers.ShouldCallByName(phase) {
		return
	}
	m.callers.Call(
		phase,
		WithCallConfigForceSync(true),
		WithCallConfigRuntimeCtx(runtimeCtx),
		WithCallConfigItems(hotCtx),
	)
}

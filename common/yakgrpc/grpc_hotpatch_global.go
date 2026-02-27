package yakgrpc

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	maxGlobalHotPatchItems            = 1
	defaultGlobalHotPatchTemplateType = "global"
	globalHotPatchCompileTimeoutSec   = 5.0
)

func (s *Server) GetGlobalHotPatchConfig(ctx context.Context, _ *ypb.Empty) (*ypb.GlobalHotPatchConfig, error) {
	return yakit.GetGlobalHotPatchConfigFromKV(s.GetProfileDatabase())
}

func (s *Server) SetGlobalHotPatchConfig(ctx context.Context, req *ypb.SetGlobalHotPatchConfigRequest) (*ypb.GlobalHotPatchConfig, error) {
	current, err := yakit.GetGlobalHotPatchConfigFromKV(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	if ev := req.GetExpectedVersion(); ev > 0 && ev != current.GetVersion() {
		return nil, utils.Errorf("global hotpatch config version conflict: expected=%d actual=%d", ev, current.GetVersion())
	}

	next, err := normalizeGlobalHotPatchConfig(req.GetConfig())
	if err != nil {
		return nil, err
	}
	next.Version = current.GetVersion() + 1

	code, err := yakit.ResolveGlobalHotPatchCode(s.GetProfileDatabase(), next)
	if err != nil {
		return nil, err
	}
	if next.GetEnabled() && strings.TrimSpace(code) != "" {
		if err := validateGlobalHotPatchCodeCompilable(code); err != nil {
			return nil, err
		}
	}

	if err := yakit.PutGlobalHotPatchConfigToKV(s.GetProfileDatabase(), next); err != nil {
		return nil, err
	}
	yakit.SetGlobalHotPatchRuntimeCache(next, code)
	return next, nil
}

func (s *Server) ResetGlobalHotPatchConfig(ctx context.Context, _ *ypb.Empty) (*ypb.GlobalHotPatchConfig, error) {
	current, err := yakit.GetGlobalHotPatchConfigFromKV(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	next := &ypb.GlobalHotPatchConfig{
		Enabled: false,
		Version: current.GetVersion() + 1,
		Items:   nil,
	}
	if err := yakit.PutGlobalHotPatchConfigToKV(s.GetProfileDatabase(), next); err != nil {
		return nil, err
	}
	yakit.SetGlobalHotPatchRuntimeCache(next, "")
	return next, nil
}

func normalizeGlobalHotPatchConfig(cfg *ypb.GlobalHotPatchConfig) (*ypb.GlobalHotPatchConfig, error) {
	if cfg == nil {
		return &ypb.GlobalHotPatchConfig{Enabled: false}, nil
	}
	next := &ypb.GlobalHotPatchConfig{Enabled: cfg.GetEnabled()}
	if !next.Enabled {
		next.Items = nil
		return next, nil
	}
	if len(cfg.GetItems()) == 0 {
		return nil, utils.Error("global hotpatch enabled but Items is empty")
	}
	if len(cfg.GetItems()) > maxGlobalHotPatchItems {
		return nil, utils.Errorf("global hotpatch items too many: %d (max=%d)", len(cfg.GetItems()), maxGlobalHotPatchItems)
	}

	src := cfg.GetItems()[0]
	if src == nil {
		return nil, utils.Error("global hotpatch item is nil")
	}
	name := strings.TrimSpace(src.GetName())
	if name == "" {
		return nil, utils.Error("global hotpatch template name is empty")
	}
	typ := strings.TrimSpace(src.GetType())
	if typ == "" {
		typ = defaultGlobalHotPatchTemplateType
	}
	next.Items = []*ypb.GlobalHotPatchTemplateRef{{
		Name:    name,
		Type:    typ,
		Enabled: true,
	}}
	return next, nil
}

func validateGlobalHotPatchCodeCompilable(code string) error {
	caller, err := yak.NewMixPluginCaller()
	if err != nil {
		return err
	}
	caller.SetLoadPluginTimeout(globalHotPatchCompileTimeoutSec)
	caller.SetCallPluginTimeout(consts.GetGlobalCallerCallPluginTimeout())
	return caller.LoadHotPatch(utils.TimeoutContextSeconds(globalHotPatchCompileTimeoutSec), nil, code)
}

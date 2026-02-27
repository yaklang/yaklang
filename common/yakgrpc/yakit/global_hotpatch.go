package yakit

import (
	"encoding/json"
	"strings"
	"sync/atomic"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type globalHotPatchRuntime struct {
	Config *ypb.GlobalHotPatchConfig
	Code   string
}

// process-wide cache; versioning is handled by the KV payload itself.
// We always store a default value to avoid atomic.Value Load panics.
var globalHotPatchRuntimeCache atomic.Value // *globalHotPatchRuntime

func init() {
	globalHotPatchRuntimeCache.Store(&globalHotPatchRuntime{Config: &ypb.GlobalHotPatchConfig{}})
	RegisterPostInitDatabaseFunction(func() error {
		db := consts.GetGormProfileDatabase()
		cfg, err := GetGlobalHotPatchConfigFromKV(db)
		if err != nil {
			log.Errorf("load global hotpatch config failed: %v", err)
			return nil
		}
		code, err := ResolveGlobalHotPatchCode(db, cfg)
		if err != nil {
			// Do not block server startup; surface by logs, and runtime sees empty code.
			log.Errorf("resolve global hotpatch code failed: %v", err)
			code = ""
		}
		SetGlobalHotPatchRuntimeCache(cfg, code)
		return nil
	}, "load-global-hotpatch-config")
}

func defaultGlobalHotPatchConfig() *ypb.GlobalHotPatchConfig {
	return &ypb.GlobalHotPatchConfig{
		Enabled: false,
		Version: 0,
		Items:   nil,
	}
}

func cloneGlobalHotPatchConfig(cfg *ypb.GlobalHotPatchConfig) *ypb.GlobalHotPatchConfig {
	if cfg == nil {
		return defaultGlobalHotPatchConfig()
	}
	out := &ypb.GlobalHotPatchConfig{
		Enabled: cfg.GetEnabled(),
		Version: cfg.GetVersion(),
	}
	if len(cfg.GetItems()) == 0 {
		return out
	}
	out.Items = make([]*ypb.GlobalHotPatchTemplateRef, 0, len(cfg.GetItems()))
	for _, item := range cfg.GetItems() {
		if item == nil {
			continue
		}
		out.Items = append(out.Items, &ypb.GlobalHotPatchTemplateRef{
			Name:    item.GetName(),
			Type:    item.GetType(),
			Enabled: item.GetEnabled(),
		})
	}
	return out
}

func GetGlobalHotPatchConfigFromKV(db *gorm.DB) (*ypb.GlobalHotPatchConfig, error) {
	if db == nil {
		return nil, utils.Error("no database set")
	}
	raw := strings.TrimSpace(GetKey(db, consts.GLOBAL_HOTPATCH_CONFIG))
	if raw == "" {
		return defaultGlobalHotPatchConfig(), nil
	}
	cfg := &ypb.GlobalHotPatchConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, utils.Errorf("unmarshal global hotpatch config failed: %v", err)
	}
	if cfg.Items == nil {
		cfg.Items = nil
	}
	return cfg, nil
}

func PutGlobalHotPatchConfigToKV(db *gorm.DB, cfg *ypb.GlobalHotPatchConfig) error {
	if db == nil {
		return utils.Error("no database set")
	}
	if cfg == nil {
		cfg = defaultGlobalHotPatchConfig()
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return utils.Errorf("marshal global hotpatch config failed: %v", err)
	}
	return SetKey(db, consts.GLOBAL_HOTPATCH_CONFIG, string(raw))
}

func ResolveGlobalHotPatchCode(db *gorm.DB, cfg *ypb.GlobalHotPatchConfig) (string, error) {
	if db == nil {
		return "", utils.Error("no database set")
	}
	if cfg == nil || !cfg.GetEnabled() {
		return "", nil
	}
	if len(cfg.GetItems()) == 0 {
		return "", utils.Error("global hotpatch enabled but Items is empty")
	}

	var selected *ypb.GlobalHotPatchTemplateRef
	for _, item := range cfg.GetItems() {
		if item == nil {
			continue
		}
		if item.GetEnabled() {
			selected = item
			break
		}
	}
	if selected == nil {
		selected = cfg.GetItems()[0]
	}

	name := strings.TrimSpace(selected.GetName())
	if name == "" {
		return "", utils.Error("global hotpatch template name is empty")
	}
	typ := strings.TrimSpace(selected.GetType())
	if typ == "" {
		typ = "global"
	}

	templates, err := QueryHotPatchTemplate(db, &ypb.HotPatchTemplateRequest{
		Name: []string{name},
		Type: typ,
	})
	if err != nil {
		return "", err
	}
	if len(templates) == 0 || templates[0] == nil {
		return "", utils.Errorf("global hotpatch template not found: name=%s type=%s", name, typ)
	}
	return templates[0].Content, nil
}

func SetGlobalHotPatchRuntimeCache(cfg *ypb.GlobalHotPatchConfig, code string) {
	globalHotPatchRuntimeCache.Store(&globalHotPatchRuntime{Config: cloneGlobalHotPatchConfig(cfg), Code: code})
}

func GetGlobalHotPatchRuntimeCache() (*ypb.GlobalHotPatchConfig, string) {
	v := globalHotPatchRuntimeCache.Load()
	if v == nil {
		return defaultGlobalHotPatchConfig(), ""
	}
	runtime, ok := v.(*globalHotPatchRuntime)
	if !ok || runtime == nil {
		return defaultGlobalHotPatchConfig(), ""
	}
	return cloneGlobalHotPatchConfig(runtime.Config), runtime.Code
}

func GetGlobalHotPatchVersionAndCode() (enabled bool, version int64, code string) {
	v := globalHotPatchRuntimeCache.Load()
	if v == nil {
		return false, 0, ""
	}
	runtime, ok := v.(*globalHotPatchRuntime)
	if !ok || runtime == nil || runtime.Config == nil {
		return false, 0, ""
	}
	return runtime.Config.GetEnabled(), runtime.Config.GetVersion(), runtime.Code
}

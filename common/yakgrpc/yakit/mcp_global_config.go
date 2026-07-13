package yakit

import (
	"encoding/json"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcpcatalog"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

var (
	cachedMCPGlobalConfig     *ypb.MCPGlobalConfig
	cachedMCPGlobalConfigLock sync.RWMutex

	mcpBuiltinToolDefaultEnableResolver func(db *gorm.DB, toolName string) (bool, error)
)

// RegisterMCPBuiltinToolDefaultEnableResolver wires builtin tool default-enable
// resolution without importing common/mcp from yakit (avoids import cycles).
func RegisterMCPBuiltinToolDefaultEnableResolver(fn func(db *gorm.DB, toolName string) (bool, error)) {
	mcpBuiltinToolDefaultEnableResolver = fn
}

func setCachedMCPGlobalConfig(cfg *ypb.MCPGlobalConfig) {
	cachedMCPGlobalConfigLock.Lock()
	defer cachedMCPGlobalConfigLock.Unlock()
	cachedMCPGlobalConfig = cloneMCPGlobalConfig(cfg)
}

func GetCachedMCPGlobalConfig() *ypb.MCPGlobalConfig {
	cachedMCPGlobalConfigLock.RLock()
	defer cachedMCPGlobalConfigLock.RUnlock()
	return cloneMCPGlobalConfig(cachedMCPGlobalConfig)
}

// SetCachedMCPGlobalConfigForTest overrides the in-memory MCP global config cache.
func SetCachedMCPGlobalConfigForTest(cfg *ypb.MCPGlobalConfig) {
	setCachedMCPGlobalConfig(cfg)
}

func cloneMCPGlobalConfig(cfg *ypb.MCPGlobalConfig) *ypb.MCPGlobalConfig {
	if cfg == nil {
		return nil
	}
	return proto.Clone(cfg).(*ypb.MCPGlobalConfig)
}

func CatalogMCPGlobalConfig() *ypb.MCPGlobalConfig {
	return &ypb.MCPGlobalConfig{
		DefaultToolSets:         append([]string{}, mcpcatalog.DefaultToolSetNames()...),
		DefaultResourceSets:     append([]string{}, mcpcatalog.DefaultResourceSetNames()...),
		EnableAIToolFramework:   false,
		EnableBridgeExternalMCP: false,
		UsesCatalogDefaults:     true,
	}
}

func HasMCPGlobalConfig(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	return GetKey(db, consts.MCP_GLOBAL_CONFIG_KEY) != ""
}

func GetMCPGlobalConfig(db *gorm.DB) (*ypb.MCPGlobalConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	if !HasMCPGlobalConfig(db) {
		return CatalogMCPGlobalConfig(), nil
	}
	raw := GetKey(db, consts.MCP_GLOBAL_CONFIG_KEY)
	if raw == "" {
		return CatalogMCPGlobalConfig(), nil
	}
	cfg := &ypb.MCPGlobalConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, err
	}
	normalizeMCPGlobalConfig(cfg)
	cfg.UsesCatalogDefaults = false
	return cfg, nil
}

func SetMCPGlobalConfig(db *gorm.DB, cfg *ypb.MCPGlobalConfig) (*ypb.MCPGlobalConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	if cfg == nil {
		return nil, utils.Error("config is nil")
	}
	normalized := cloneMCPGlobalConfig(cfg)
	normalizeMCPGlobalConfig(normalized)
	normalized.UsesCatalogDefaults = false

	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if err := SetKey(db, consts.MCP_GLOBAL_CONFIG_KEY, string(data)); err != nil {
		return nil, err
	}
	ApplyMCPGlobalConfig(normalized)
	return normalized, nil
}

func ResetMCPGlobalConfig(db *gorm.DB) (*ypb.MCPGlobalConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	DelKey(db, consts.MCP_GLOBAL_CONFIG_KEY)
	cfg := CatalogMCPGlobalConfig()
	ApplyMCPGlobalConfig(cfg)
	return cfg, nil
}

func ApplyMCPGlobalConfig(cfg *ypb.MCPGlobalConfig) {
	if cfg == nil {
		setCachedMCPGlobalConfig(CatalogMCPGlobalConfig())
		return
	}
	setCachedMCPGlobalConfig(cfg)
}

func EffectiveDefaultMCPToolSets(db *gorm.DB) ([]string, error) {
	cfg, err := resolveMCPGlobalConfig(db)
	if err != nil {
		return nil, err
	}
	return append([]string{}, cfg.DefaultToolSets...), nil
}

func EffectiveDefaultMCPResourceSets(db *gorm.DB) ([]string, error) {
	cfg, err := resolveMCPGlobalConfig(db)
	if err != nil {
		return nil, err
	}
	return append([]string{}, cfg.DefaultResourceSets...), nil
}

func EffectiveDefaultMCPToolSetMap(db *gorm.DB) (map[string]struct{}, error) {
	sets, err := EffectiveDefaultMCPToolSets(db)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(sets))
	for _, name := range sets {
		out[name] = struct{}{}
	}
	return out, nil
}

func IsBuiltinToolInEffectiveDefaultSets(db *gorm.DB, toolName string) (bool, error) {
	if mcpBuiltinToolDefaultEnableResolver == nil {
		return false, nil
	}
	return mcpBuiltinToolDefaultEnableResolver(db, toolName)
}

func IsToolSetEnabledByDefault(db *gorm.DB, setName string) (bool, error) {
	defaultSets, err := EffectiveDefaultMCPToolSetMap(db)
	if err != nil {
		return false, err
	}
	_, ok := defaultSets[setName]
	return ok, nil
}

func resolveMCPGlobalConfig(db *gorm.DB) (*ypb.MCPGlobalConfig, error) {
	if cached := GetCachedMCPGlobalConfig(); cached != nil {
		return cached, nil
	}
	cfg, err := GetMCPGlobalConfig(db)
	if err != nil {
		return nil, err
	}
	setCachedMCPGlobalConfig(cfg)
	return cfg, nil
}

func normalizeMCPGlobalConfig(cfg *ypb.MCPGlobalConfig) {
	if cfg == nil {
		return
	}
	cfg.DefaultToolSets = dedupeNonEmptyStrings(cfg.DefaultToolSets)
	cfg.DefaultResourceSets = dedupeNonEmptyStrings(cfg.DefaultResourceSets)
	if len(cfg.DefaultToolSets) == 0 {
		cfg.DefaultToolSets = append([]string{}, mcpcatalog.DefaultToolSetNames()...)
	}
	if len(cfg.DefaultResourceSets) == 0 {
		cfg.DefaultResourceSets = append([]string{}, mcpcatalog.DefaultResourceSetNames()...)
	}
}

func dedupeNonEmptyStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

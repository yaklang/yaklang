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
	mcpToolSetNamesValidator            func(names []string) error
	mcpResourceSetNamesValidator        func(names []string) error
)

// RegisterMCPBuiltinToolDefaultEnableResolver wires builtin tool default-enable
// resolution without importing common/mcp from yakit (avoids import cycles).
func RegisterMCPBuiltinToolDefaultEnableResolver(fn func(db *gorm.DB, toolName string) (bool, error)) {
	mcpBuiltinToolDefaultEnableResolver = fn
}

// RegisterMCPGlobalConfigValidators wires tool/resource set name validation
// without importing common/mcp from yakit.
func RegisterMCPGlobalConfigValidators(toolSets, resourceSets func(names []string) error) {
	mcpToolSetNamesValidator = toolSets
	mcpResourceSetNamesValidator = resourceSets
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
		cfg := CatalogMCPGlobalConfig()
		setCachedMCPGlobalConfig(cfg)
		return cfg, nil
	}
	raw := GetKey(db, consts.MCP_GLOBAL_CONFIG_KEY)
	if raw == "" {
		cfg := CatalogMCPGlobalConfig()
		setCachedMCPGlobalConfig(cfg)
		return cfg, nil
	}
	cfg := &ypb.MCPGlobalConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, err
	}
	normalizeMCPGlobalConfig(cfg)
	if cfg.GetUsesCatalogDefaults() {
		cfg.DefaultToolSets = append([]string{}, mcpcatalog.DefaultToolSetNames()...)
		cfg.DefaultResourceSets = append([]string{}, mcpcatalog.DefaultResourceSetNames()...)
	} else {
		cfg.UsesCatalogDefaults = false
	}
	setCachedMCPGlobalConfig(cfg)
	return cfg, nil
}

func SetMCPGlobalConfig(db *gorm.DB, cfg *ypb.MCPGlobalConfig) (*ypb.MCPGlobalConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	if cfg == nil {
		return nil, utils.Error("config is nil")
	}

	inputToolSets := dedupeNonEmptyStrings(cfg.DefaultToolSets)
	inputResourceSets := dedupeNonEmptyStrings(cfg.DefaultResourceSets)
	clearRequest := len(inputToolSets) == 0 &&
		len(inputResourceSets) == 0 &&
		!cfg.GetEnableAIToolFramework() &&
		!cfg.GetEnableBridgeExternalMCP()
	if clearRequest {
		return ResetMCPGlobalConfig(db)
	}

	normalized := cloneMCPGlobalConfig(cfg)
	followCatalogSets := len(inputToolSets) == 0 && len(inputResourceSets) == 0
	normalizeMCPGlobalConfig(normalized)
	if followCatalogSets {
		normalized.UsesCatalogDefaults = true
		normalized.DefaultToolSets = append([]string{}, mcpcatalog.DefaultToolSetNames()...)
		normalized.DefaultResourceSets = append([]string{}, mcpcatalog.DefaultResourceSetNames()...)
	} else {
		normalized.UsesCatalogDefaults = false
	}

	if err := validateMCPGlobalConfigSets(normalized); err != nil {
		return nil, err
	}

	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if err := SetKey(db, consts.MCP_GLOBAL_CONFIG_KEY, string(data)); err != nil {
		return nil, err
	}
	ApplyMCPGlobalConfig(normalized)
	if err := SyncBuiltinMCPClientToolEnablesToDefaults(db); err != nil {
		return nil, err
	}
	return normalized, nil
}

func ResetMCPGlobalConfig(db *gorm.DB) (*ypb.MCPGlobalConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	DelKey(db, consts.MCP_GLOBAL_CONFIG_KEY)
	cfg := CatalogMCPGlobalConfig()
	ApplyMCPGlobalConfig(cfg)
	if err := SyncBuiltinMCPClientToolEnablesToDefaults(db); err != nil {
		return nil, err
	}
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

// resolveMCPGlobalConfig always reloads from DB so StartMcpServer / GetToolSetList /
// CLI share one source of truth and avoid stale in-memory cache.
func resolveMCPGlobalConfig(db *gorm.DB) (*ypb.MCPGlobalConfig, error) {
	return GetMCPGlobalConfig(db)
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

func validateMCPGlobalConfigSets(cfg *ypb.MCPGlobalConfig) error {
	if cfg == nil {
		return nil
	}
	if mcpToolSetNamesValidator != nil {
		if err := mcpToolSetNamesValidator(cfg.DefaultToolSets); err != nil {
			return err
		}
	} else if err := validateToolSetNamesAgainstCatalog(cfg.DefaultToolSets); err != nil {
		return err
	}
	if mcpResourceSetNamesValidator != nil {
		if err := mcpResourceSetNamesValidator(cfg.DefaultResourceSets); err != nil {
			return err
		}
	} else if err := validateResourceSetNamesAgainstCatalog(cfg.DefaultResourceSets); err != nil {
		return err
	}
	return nil
}

func validateToolSetNamesAgainstCatalog(names []string) error {
	known := make(map[string]struct{}, len(mcpcatalog.AllToolSetNames()))
	for _, name := range mcpcatalog.AllToolSetNames() {
		known[name] = struct{}{}
	}
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, ok := known[name]; !ok {
			return utils.Errorf("undefined tool set: %s", name)
		}
	}
	return nil
}

func validateResourceSetNamesAgainstCatalog(names []string) error {
	known := make(map[string]struct{}, len(mcpcatalog.DefaultResourceSetNames()))
	for _, name := range mcpcatalog.DefaultResourceSetNames() {
		known[name] = struct{}{}
	}
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, ok := known[name]; !ok {
			return utils.Errorf("undefined resource set: %s", name)
		}
	}
	return nil
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

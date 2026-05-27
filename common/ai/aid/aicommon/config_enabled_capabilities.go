package aicommon

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	EnabledCapabilityTypeTool    = "tool"
	EnabledCapabilityTypeSkill   = "skill"
	EnabledCapabilityTypePlugin  = "plugin"
	EnabledCapabilityTypeForge   = "forge"
	EnabledCapabilityTypeMCPTool = "mcp_tool"

	HotPatchType_EnabledCapabilities = "EnabledCapabilities"
)

// EnabledCapability describes a capability to preload at startup or via hot patch.
type EnabledCapability struct {
	Name string `json:"name"`
	Type string `json:"type"` // tool | skill | plugin | forge | mcp_tool
}

type skillHotloadHandler func(skillNames []string)
type forgeHotloadHandler func(forgeNames []string)

func normalizeEnabledCapabilityType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case EnabledCapabilityTypeTool, "tools":
		return EnabledCapabilityTypeTool
	case EnabledCapabilityTypeSkill, "skills":
		return EnabledCapabilityTypeSkill
	case EnabledCapabilityTypePlugin, "plugins", "yakit_plugin", "yak_plugin":
		return EnabledCapabilityTypePlugin
	case EnabledCapabilityTypeForge, "forges", "blueprint", "blueprints":
		return EnabledCapabilityTypeForge
	case EnabledCapabilityTypeMCPTool, "mcp", "mcp-tool", "mcptool":
		return EnabledCapabilityTypeMCPTool
	default:
		return ""
	}
}

func normalizeEnabledCapabilities(caps []EnabledCapability) []EnabledCapability {
	if len(caps) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(caps))
	result := make([]EnabledCapability, 0, len(caps))
	for _, cap := range caps {
		name := strings.TrimSpace(cap.Name)
		capType := normalizeEnabledCapabilityType(cap.Type)
		if name == "" || capType == "" {
			continue
		}
		key := capType + ":" + name
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, EnabledCapability{Name: name, Type: capType})
	}
	return result
}

// ParseEnabledCapabilitiesFromProto converts proto entries to normalized capabilities.
func ParseEnabledCapabilitiesFromProto(params *ypb.AIStartParams) []EnabledCapability {
	if params == nil || len(params.GetEnabledCapabilities()) == 0 {
		return nil
	}
	raw := make([]EnabledCapability, 0, len(params.GetEnabledCapabilities()))
	for _, item := range params.GetEnabledCapabilities() {
		if item == nil {
			continue
		}
		raw = append(raw, EnabledCapability{
			Name: item.GetName(),
			Type: item.GetType(),
		})
	}
	return normalizeEnabledCapabilities(raw)
}

func enabledCapabilitiesToProto(caps []EnabledCapability) []*ypb.AIEnabledCapability {
	if len(caps) == 0 {
		return nil
	}
	result := make([]*ypb.AIEnabledCapability, 0, len(caps))
	for _, cap := range caps {
		result = append(result, &ypb.AIEnabledCapability{
			Name: cap.Name,
			Type: cap.Type,
		})
	}
	return result
}

func (c *Config) setEnabledCapabilitiesLocked(caps []EnabledCapability) {
	c.enabledCapabilities = normalizeEnabledCapabilities(caps)
}

func (c *Config) GetEnabledCapabilities() []EnabledCapability {
	if c == nil {
		return nil
	}
	c.m.Lock()
	defer c.m.Unlock()
	if len(c.enabledCapabilities) == 0 {
		return nil
	}
	result := make([]EnabledCapability, len(c.enabledCapabilities))
	copy(result, c.enabledCapabilities)
	return result
}

func capabilityNamesByType(caps []EnabledCapability, capType string) []string {
	names := make([]string, 0)
	for _, cap := range caps {
		if cap.Type == capType {
			names = append(names, cap.Name)
		}
	}
	return names
}

func (c *Config) GetEnabledSkillNames() []string {
	return capabilityNamesByType(c.GetEnabledCapabilities(), EnabledCapabilityTypeSkill)
}

func (c *Config) GetEnabledForgeNames() []string {
	return capabilityNamesByType(c.GetEnabledCapabilities(), EnabledCapabilityTypeForge)
}

// WithEnabledCapabilities stores startup/hotpatch enabled capabilities and applies immediate entries.
func WithEnabledCapabilities(caps ...EnabledCapability) ConfigOption {
	return func(c *Config) error {
		if c == nil {
			return nil
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		prevSkillNames := capabilityNamesByType(c.enabledCapabilities, EnabledCapabilityTypeSkill)
		prevForgeNames := capabilityNamesByType(c.enabledCapabilities, EnabledCapabilityTypeForge)
		c.setEnabledCapabilitiesLocked(caps)
		nextSkillNames := capabilityNamesByType(c.enabledCapabilities, EnabledCapabilityTypeSkill)
		nextForgeNames := capabilityNamesByType(c.enabledCapabilities, EnabledCapabilityTypeForge)
		tmReady := c.AiToolManager != nil
		c.m.Unlock()

		if tmReady {
			if err := c.applyEnabledImmediateCapabilities(); err != nil {
				return err
			}
		}
		c.notifySkillHotload(diffNames(prevSkillNames, nextSkillNames))
		c.notifyForgeHotload(diffNames(prevForgeNames, nextForgeNames))
		return nil
	}
}

func diffNames(prev, next []string) []string {
	if len(next) == 0 {
		return nil
	}
	prevSet := make(map[string]struct{}, len(prev))
	for _, name := range prev {
		prevSet[name] = struct{}{}
	}
	added := make([]string, 0)
	for _, name := range next {
		if _, ok := prevSet[name]; !ok {
			added = append(added, name)
		}
	}
	return added
}

func (c *Config) applyEnabledImmediateCapabilities() error {
	if c == nil {
		return nil
	}
	caps := c.GetEnabledCapabilities()
	if len(caps) == 0 {
		return nil
	}

	tm := c.GetAiToolManager()
	if tm == nil {
		return utils.Error("ai tool manager is nil")
	}

	var appendTools []*aitool.Tool
	for _, cap := range caps {
		switch cap.Type {
		case EnabledCapabilityTypeTool:
			tm.EnableTool(cap.Name)
		case EnabledCapabilityTypePlugin:
			tool, err := loadPluginAsTool(cap.Name)
			if err != nil {
				log.Warnf("enabled capability plugin %q load failed: %v", cap.Name, err)
				continue
			}
			if tool != nil {
				appendTools = append(appendTools, tool)
			}
		case EnabledCapabilityTypeMCPTool:
			tools, err := aitool.LoadAIToolsFromMCPCapability(consts.GetGormProfileDatabase(), c.Ctx, cap.Name)
			if err != nil {
				log.Warnf("enabled capability mcp_tool %q load failed: %v", cap.Name, err)
				continue
			}
			for _, tool := range tools {
				if tool != nil {
					appendTools = append(appendTools, tool)
				}
			}
		}
	}

	if len(appendTools) > 0 {
		if err := tm.AppendTools(appendTools...); err != nil {
			return err
		}
		for _, tool := range appendTools {
			tm.EnableTool(tool.Name)
		}
	}
	return nil
}

func loadPluginAsTool(name string) (*aitool.Tool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, utils.Error("plugin name is empty")
	}
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Error("profile database is nil")
	}
	script, err := yakit.GetYakScriptByNameForAI(db, name)
	if err != nil {
		return nil, err
	}
	return yakscripttools.ConvertYakScriptPlugin(script)
}

func (c *Config) SetSkillHotloadHandler(handler skillHotloadHandler) {
	if c == nil {
		return
	}
	if c.m == nil {
		c.m = &sync.Mutex{}
	}
	c.m.Lock()
	defer c.m.Unlock()
	c.skillHotloadHandler = handler
}

func (c *Config) SetForgeHotloadHandler(handler forgeHotloadHandler) {
	if c == nil {
		return
	}
	if c.m == nil {
		c.m = &sync.Mutex{}
	}
	c.m.Lock()
	defer c.m.Unlock()
	c.forgeHotloadHandler = handler
}

func (c *Config) notifySkillHotload(skillNames []string) {
	if c == nil || len(skillNames) == 0 {
		return
	}
	c.m.Lock()
	handler := c.skillHotloadHandler
	c.m.Unlock()
	if handler != nil {
		handler(skillNames)
	}
}

func (c *Config) notifyForgeHotload(forgeNames []string) {
	if c == nil || len(forgeNames) == 0 {
		return
	}
	c.m.Lock()
	handler := c.forgeHotloadHandler
	c.m.Unlock()
	if handler != nil {
		handler(forgeNames)
	}
}

// MergeEnabledCapabilitiesHotpatch merges enabled capabilities from base and patch start params.
func MergeEnabledCapabilitiesHotpatch(base *ypb.AIStartParams, patch *ypb.AIStartParams) []*ypb.AIEnabledCapability {
	if patch == nil || len(patch.GetEnabledCapabilities()) == 0 {
		return nil
	}
	merged := make([]EnabledCapability, 0)
	if base != nil {
		merged = append(merged, ParseEnabledCapabilitiesFromProto(base)...)
	}
	merged = append(merged, ParseEnabledCapabilitiesFromProto(patch)...)
	return enabledCapabilitiesToProto(normalizeEnabledCapabilities(merged))
}

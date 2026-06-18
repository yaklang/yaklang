package aicommon

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const CapabilityInventoryNodeID = "capability_inventory"

// CapabilityInventoryItem is a flattened inventory record for UI consumption.
// It does not replace CapabilityInventoryPayload; it is emitted separately.
type CapabilityInventoryItem struct {
	Name        string `json:"name"`
	VerboseName string `json:"verbose_name,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`           // mcp | aitool | plugin | skill | forge
	Stage       string `json:"stage"`          // loaded | metadata | schema | description
	Position    string `json:"position"`       // FrozenBlock | Dynamic | SemiDynamic
	IsFixed     bool   `json:"is_fixed"`       // fixed inventory, cannot be changed by hotpatch
	Data        any    `json:"data,omitempty"` // extra arbitrary payload
}

type CapabilityInventoryToolItem struct {
	Name        string `json:"name"`
	VerboseName string `json:"verbose_name,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category"`
}

// Skill load states for capability_inventory.skills entries (SkillLoadState field).
const (
	CapabilityInventorySkillLoadMetadata = "metadata" // in prompt Available Skills registry only
	CapabilityInventorySkillLoadLoaded   = "loaded"   // fully loaded into SKILLS_CONTEXT
)

type CapabilityInventoryNamedItem struct {
	Name        string `json:"name"`
	VerboseName string `json:"verbose_name,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	// SkillLoadState is set for category "skill" only: "metadata" | "loaded".
	SkillLoadState string `json:"skill_load_state,omitempty"`
}

type CapabilityInventorySection struct {
	Tools      []CapabilityInventoryToolItem  `json:"tools,omitempty"`
	Skills     []CapabilityInventoryNamedItem `json:"skills,omitempty"`
	Forges     []CapabilityInventoryNamedItem `json:"forges,omitempty"`
	MCPServers []CapabilityInventoryNamedItem `json:"mcp_servers,omitempty"`
}

type CapabilityInventoryPayload struct {
	Fixed   CapabilityInventorySection `json:"fixed"`
	Dynamic CapabilityInventorySection `json:"dynamic"`
}

// CapabilityInventoryLoopContext 描述 loop prompt 构建时的工具上下文, 以及运行时
// 动态能力. loop 为 nil 时表示无 loop 上下文 (如 coordinator 初始化).
type CapabilityInventoryLoopContext interface {
	PromptCandidateTools() []*aitool.Tool
	ScenarioToolWhitelist() []string
	AllowToolCall() bool
	DynamicExtraTools() []*aitool.Tool
	DynamicForges() []CapabilityInventoryNamedItem
	// InventorySkills lists registry + loaded skills with SkillLoadState (see constants above).
	InventorySkills() []CapabilityInventoryNamedItem
}

// ConfigPromptCapabilityLoopContext 表示无 loop 时的 prompt 工具上下文,
// 与 ReActLoop.toolsGetter == nil 时 generateLoopPrompt 行为一致.
type ConfigPromptCapabilityLoopContext struct{}

func (ConfigPromptCapabilityLoopContext) PromptCandidateTools() []*aitool.Tool { return nil }
func (ConfigPromptCapabilityLoopContext) ScenarioToolWhitelist() []string      { return nil }
func (ConfigPromptCapabilityLoopContext) AllowToolCall() bool                  { return true }
func (ConfigPromptCapabilityLoopContext) DynamicExtraTools() []*aitool.Tool    { return nil }
func (ConfigPromptCapabilityLoopContext) DynamicForges() []CapabilityInventoryNamedItem {
	return nil
}
func (ConfigPromptCapabilityLoopContext) InventorySkills() []CapabilityInventoryNamedItem { return nil }

func resolveCapabilityInventoryLoopContext(loop CapabilityInventoryLoopContext) CapabilityInventoryLoopContext {
	if loop != nil {
		return loop
	}
	return ConfigPromptCapabilityLoopContext{}
}

func classifyToolCategory(tool *aitool.Tool) string {
	if tool == nil {
		return "tool"
	}
	name := strings.TrimSpace(tool.Name)
	switch {
	case strings.HasPrefix(name, "mcp_"):
		return "mcp"
	case strings.Contains(strings.ToLower(tool.VerboseName), "yak plugin"),
		strings.Contains(strings.ToLower(tool.VerboseName), "core tool"),
		strings.Contains(strings.ToLower(tool.Description), "native yak plugin"),
		strings.Contains(strings.ToLower(tool.Description), "mitm plugin"),
		strings.Contains(strings.ToLower(tool.Description), "port-scan plugin"):
		return "yak_plugin"
	default:
		return "tool"
	}
}

func collectEnabledMCPServers(disallow bool) []CapabilityInventoryNamedItem {
	if disallow {
		return nil
	}
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil
	}
	result := make([]CapabilityInventoryNamedItem, 0)
	for server := range yakit.YieldEnabledMCPServers(context.Background(), db) {
		if server == nil || strings.TrimSpace(server.Name) == "" {
			continue
		}
		desc := server.URL
		if desc == "" {
			desc = server.Command
		}
		result = append(result, CapabilityInventoryNamedItem{
			Name:        server.Name,
			VerboseName: server.Name,
			Description: utils.ShrinkString(desc, 200),
			Category:    "mcp_server",
		})
	}
	return result
}

func loopPromptCandidateTools(loop CapabilityInventoryLoopContext) []*aitool.Tool {
	if loop == nil {
		return nil
	}
	return loop.PromptCandidateTools()
}

// BuildCapabilityInventoryPayload builds the legacy capability_inventory payload
// by flattening session_snapshot capabilities and supplementing MCP servers.
func BuildCapabilityInventoryPayload(cfg *Config, loop CapabilityInventoryLoopContext) CapabilityInventoryPayload {
	if cfg == nil {
		return CapabilityInventoryPayload{}
	}
	items := BuildCapabilityInventoryItems(cfg, loop)
	return CapabilityInventoryPayloadFromItems(items, cfg)
}

// CapabilityInventoryPayloadFromItems maps flattened session_snapshot capabilities
// into the legacy fixed/dynamic sections. Items in Dynamic prompt position go to
// Dynamic; FrozenBlock and SemiDynamic go to Fixed.
func CapabilityInventoryPayloadFromItems(items []CapabilityInventoryItem, cfg *Config) CapabilityInventoryPayload {
	payload := CapabilityInventoryPayload{}
	seenMCPServers := make(map[string]struct{})

	for _, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		dynamic := item.Position == CapabilityInventoryPositionDynamic
		switch item.Type {
		case "skill":
			named := capabilityInventoryItemToNamed(item, "skill")
			named.SkillLoadState = skillLoadStateFromItem(item)
			if dynamic {
				payload.Dynamic.Skills = append(payload.Dynamic.Skills, named)
			} else {
				payload.Fixed.Skills = append(payload.Fixed.Skills, named)
			}
		case "forge":
			named := capabilityInventoryItemToNamed(item, "forge")
			if dynamic {
				payload.Dynamic.Forges = append(payload.Dynamic.Forges, named)
			} else {
				payload.Fixed.Forges = append(payload.Fixed.Forges, named)
			}
		case "mcp_server":
			named := capabilityInventoryItemToNamed(item, "mcp_server")
			payload.Fixed.MCPServers = append(payload.Fixed.MCPServers, named)
			seenMCPServers[item.Name] = struct{}{}
		default:
			tool := capabilityInventoryItemToTool(item)
			if dynamic {
				payload.Dynamic.Tools = append(payload.Dynamic.Tools, tool)
			} else {
				payload.Fixed.Tools = append(payload.Fixed.Tools, tool)
			}
		}
	}

	if cfg != nil {
		for _, server := range collectEnabledMCPServers(cfg.DisallowMCPServers) {
			if _, ok := seenMCPServers[server.Name]; ok {
				continue
			}
			payload.Fixed.MCPServers = append(payload.Fixed.MCPServers, server)
		}
	}
	return payload
}

func capabilityInventoryItemToTool(item CapabilityInventoryItem) CapabilityInventoryToolItem {
	return CapabilityInventoryToolItem{
		Name:        item.Name,
		VerboseName: item.VerboseName,
		Description: item.Description,
		Category:    inventoryItemTypeToToolCategory(item.Type),
	}
}

func capabilityInventoryItemToNamed(item CapabilityInventoryItem, category string) CapabilityInventoryNamedItem {
	return CapabilityInventoryNamedItem{
		Name:        item.Name,
		VerboseName: item.VerboseName,
		Description: item.Description,
		Category:    category,
	}
}

func inventoryItemTypeToToolCategory(typ string) string {
	switch typ {
	case "mcp":
		return "mcp"
	case "plugin":
		return "yak_plugin"
	default:
		return "tool"
	}
}

func skillLoadStateFromItem(item CapabilityInventoryItem) string {
	if item.Stage == CapabilityInventoryStageLoaded {
		return CapabilityInventorySkillLoadLoaded
	}
	if data, ok := item.Data.(map[string]any); ok {
		if state, ok := data["skill_load_state"].(string); ok && strings.TrimSpace(state) != "" {
			return state
		}
	}
	return CapabilityInventorySkillLoadMetadata
}

func resolveCapabilityInventorySkills(cfg *Config, loop CapabilityInventoryLoopContext) []CapabilityInventoryNamedItem {
	skills := loop.InventorySkills()
	if len(skills) > 0 {
		return skills
	}
	if cfg == nil {
		return nil
	}
	return BuildInventorySkillsFromLoader(cfg.GetSkillLoader(), nil)
}

// BuildBaseCapabilityInventoryPayload 保留给 coordinator 初始化 emit.
func BuildBaseCapabilityInventoryPayload(cfg *Config) CapabilityInventoryPayload {
	return BuildCapabilityInventoryPayload(cfg, nil)
}

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

type CapabilityInventoryToolItem struct {
	Name        string   `json:"name"`
	VerboseName string   `json:"verbose_name,omitempty"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category"`
	Keywords    []string `json:"keywords,omitempty"`
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
func (ConfigPromptCapabilityLoopContext) ScenarioToolWhitelist() []string     { return nil }
func (ConfigPromptCapabilityLoopContext) AllowToolCall() bool                 { return true }
func (ConfigPromptCapabilityLoopContext) DynamicExtraTools() []*aitool.Tool   { return nil }
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

func convertToolItem(tool *aitool.Tool) CapabilityInventoryToolItem {
	if tool == nil {
		return CapabilityInventoryToolItem{}
	}
	return CapabilityInventoryToolItem{
		Name:        tool.Name,
		VerboseName: tool.VerboseName,
		Description: tool.Description,
		Category:    classifyToolCategory(tool),
		Keywords:    tool.Keywords,
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

// BuildCapabilityInventoryPayload 与 loop prompt 构建使用同一套 Tool Inventory
// 候选工具解析与筛选逻辑; loop 为 nil 时 fallback 到 ConfigPromptCapabilityLoopContext
// (等价于 toolsGetter 为空时的 generateLoopPrompt 路径).
func BuildCapabilityInventoryPayload(cfg *Config, loop CapabilityInventoryLoopContext) CapabilityInventoryPayload {
	payload := CapabilityInventoryPayload{}
	if cfg == nil {
		return payload
	}

	loop = resolveCapabilityInventoryLoopContext(loop)
	tools := ResolveLoopPromptCandidateTools(cfg, loop)
	selection := ResolvePromptToolInventory(cfg, tools, loop.ScenarioToolWhitelist(), loop.AllowToolCall())
	inventoryTools := selection.DisplayTools

	fixedTools := make([]CapabilityInventoryToolItem, 0, len(inventoryTools))
	dynamicTools := make([]CapabilityInventoryToolItem, 0)
	seenDynamic := make(map[string]struct{})

	for _, tool := range inventoryTools {
		item := convertToolItem(tool)
		if IsFixedInventoryTool(tool.Name) {
			fixedTools = append(fixedTools, item)
			continue
		}
		dynamicTools = append(dynamicTools, item)
		seenDynamic[tool.Name] = struct{}{}
	}

	for _, tool := range loop.DynamicExtraTools() {
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		if IsFixedInventoryTool(tool.Name) {
			continue
		}
		if _, ok := seenDynamic[tool.Name]; ok {
			continue
		}
		dynamicTools = append(dynamicTools, convertToolItem(tool))
		seenDynamic[tool.Name] = struct{}{}
	}

	payload.Fixed.Tools = fixedTools
	payload.Fixed.MCPServers = collectEnabledMCPServers(cfg.DisallowMCPServers)
	payload.Dynamic.Tools = dynamicTools
	payload.Dynamic.Forges = loop.DynamicForges()
	payload.Dynamic.Skills = resolveCapabilityInventorySkills(cfg, loop)
	return payload
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

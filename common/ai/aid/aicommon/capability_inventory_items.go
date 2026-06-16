package aicommon

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

const (
	CapabilityInventoryPositionFrozenBlock = "FrozenBlock"
	CapabilityInventoryPositionDynamic     = "Dynamic"
	CapabilityInventoryPositionSemiDynamic = "SemiDynamic"

	CapabilityInventoryStageLoaded      = "loaded"
	CapabilityInventoryStageMetadata    = "metadata"
	CapabilityInventoryStageSchema      = "schema"
	CapabilityInventoryStageDescription = "description"
)

func toolInventoryTypeAndStage(tool *aitool.Tool) (typ string, stage string) {
	if tool == nil {
		return "aitool", CapabilityInventoryStageMetadata
	}
	switch classifyToolCategory(tool) {
	case "mcp":
		typ = "mcp"
	case "yak_plugin":
		typ = "plugin"
	default:
		typ = "aitool"
	}

	// Tool schema stage: if inputSchema has properties, treat as schema-loaded.
	// MCP stubs and some placeholder tools may only have description.
	if tool.Tool != nil {
		props := tool.Tool.InputSchema.Properties
		if props != nil && props.Len() > 0 {
			return typ, CapabilityInventoryStageSchema
		}
	}
	if strings.TrimSpace(tool.Description) != "" {
		return typ, CapabilityInventoryStageDescription
	}
	return typ, CapabilityInventoryStageMetadata
}

func BuildCapabilityInventoryItems(cfg *Config, loop CapabilityInventoryLoopContext) []CapabilityInventoryItem {
	if cfg == nil {
		return nil
	}
	loop = resolveCapabilityInventoryLoopContext(loop)

	items := make([]CapabilityInventoryItem, 0)

	// Tools from prompt inventory (FrozenBlock)
	tools := ResolveLoopPromptCandidateTools(cfg, loop)
	selection := ResolvePromptToolInventory(cfg, tools, loop.ScenarioToolWhitelist(), loop.AllowToolCall())
	for _, t := range selection.DisplayTools {
		if t == nil || strings.TrimSpace(t.Name) == "" {
			continue
		}
		typ, stage := toolInventoryTypeAndStage(t)
		items = append(items, CapabilityInventoryItem{
			Name:        t.Name,
			VerboseName: t.VerboseName,
			Description: t.Description,
			Type:        typ,
			Stage:       stage,
			Position:    CapabilityInventoryPositionFrozenBlock,
			IsFixed:     IsFixedInventoryTool(t.Name),
		})
	}

	// Extra tools (Dynamic)
	for _, t := range loop.DynamicExtraTools() {
		if t == nil || strings.TrimSpace(t.Name) == "" {
			continue
		}
		typ, stage := toolInventoryTypeAndStage(t)
		items = append(items, CapabilityInventoryItem{
			Name:        t.Name,
			VerboseName: t.VerboseName,
			Description: t.Description,
			Type:        typ,
			Stage:       stage,
			Position:    CapabilityInventoryPositionDynamic,
			IsFixed:     false,
		})
	}

	// Skills (SemiDynamic if loaded, Dynamic if only meta suggested)
	for _, s := range resolveCapabilityInventorySkills(cfg, loop) {
		if strings.TrimSpace(s.Name) == "" {
			continue
		}
		stage := CapabilityInventoryStageMetadata
		if s.SkillLoadState == CapabilityInventorySkillLoadLoaded {
			stage = CapabilityInventoryStageLoaded
		}
		pos := CapabilityInventoryPositionSemiDynamic
		if stage == CapabilityInventoryStageMetadata {
			// skills that exist only in registry can be treated as semi-dynamic still,
			// but keep consistent UI expectations by tagging as SemiDynamic.
			pos = CapabilityInventoryPositionSemiDynamic
		}
		items = append(items, CapabilityInventoryItem{
			Name:        s.Name,
			VerboseName: s.VerboseName,
			Description: s.Description,
			Type:        "skill",
			Stage:       stage,
			Position:    pos,
			IsFixed:     false,
			Data: map[string]any{
				"skill_load_state": s.SkillLoadState,
			},
		})
	}

	// Forges (Dynamic)
	for _, f := range loop.DynamicForges() {
		if strings.TrimSpace(f.Name) == "" {
			continue
		}
		items = append(items, CapabilityInventoryItem{
			Name:        f.Name,
			VerboseName: f.VerboseName,
			Description: f.Description,
			Type:        "forge",
			Stage:       CapabilityInventoryStageMetadata,
			Position:    CapabilityInventoryPositionDynamic,
			IsFixed:     false,
		})
	}

	return items
}

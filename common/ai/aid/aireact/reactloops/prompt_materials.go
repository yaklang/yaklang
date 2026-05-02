package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// LoopPromptBaseMaterials contains pre-rendered prompt ingredients supplied by
// the runtime so the loop can assemble prompt sections without reverse-parsing
// a monolithic background template.
type LoopPromptBaseMaterials struct {
	Nonce              string
	Language           string
	TaskType           string
	ForgeName          string
	AllowPlanAndExec   bool
	AllowToolCall      bool
	HasLoadCapability  bool
	ShowForgeInventory bool
	CurrentTime        string
	OSArch             string
	WorkingDir         string
	WorkingDirGlance   string
	AutoContext        string
	UserHistory        string
	ToolsCount         int
	TopToolsCount      int
	TopTools           []*aitool.Tool
	HasMoreTools       bool
	AIForgeList        string
	Timeline           string
}

type LoopPromptAssemblyInput = aicommon.LoopPromptAssemblyInput

type LoopPromptAssemblyResult = aicommon.LoopPromptAssemblyResult

type PromptPrefixMaterials struct {
	Nonce             string
	AllowToolCall     bool
	AllowPlanAndExec  bool
	HasLoadCapability bool
	TaskInstruction   string
	OutputExample     string

	ToolInventory  bool
	ToolsCount     int
	TopToolsCount  int
	TopTools       []*aitool.Tool
	HasMoreTools   bool
	ForgeInventory bool
	AIForgeList    string
	SkillsContext  string
	Schema         string

	Timeline         string
	CurrentTime      string
	Workspace        bool
	OSArch           string
	WorkingDir       string
	WorkingDirGlance string
}

func (m *PromptPrefixMaterials) HighStaticData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"AllowToolCall":     m.AllowToolCall,
		"AllowPlanAndExec":  m.AllowPlanAndExec,
		"HasLoadCapability": m.HasLoadCapability,
		"TaskInstruction":   m.TaskInstruction,
		"OutputExample":     m.OutputExample,
	}
}

func (m *PromptPrefixMaterials) SemiDynamicData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"ToolInventory":  m.ToolInventory,
		"ToolsCount":     m.ToolsCount,
		"TopToolsCount":  m.TopToolsCount,
		"TopTools":       m.TopTools,
		"HasMoreTools":   m.HasMoreTools,
		"ForgeInventory": m.ForgeInventory,
		"AIForgeList":    m.AIForgeList,
		"SkillsContext":  m.SkillsContext,
		"Schema":         m.Schema,
	}
}

func (m *PromptPrefixMaterials) TimelineData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"Timeline":         m.Timeline,
		"CurrentTime":      m.CurrentTime,
		"Workspace":        m.Workspace,
		"OSArch":           m.OSArch,
		"WorkingDir":       m.WorkingDir,
		"WorkingDirGlance": m.WorkingDirGlance,
	}
}

type PromptPrefixAssemblyResult struct {
	Prompt      string
	HighStatic  string
	SemiDynamic string
	Timeline    string
	Sections    []*PromptSectionObservation
}

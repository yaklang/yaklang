package reactloops

import "github.com/yaklang/yaklang/common/ai/aid/aitool"

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

type LoopPromptAssemblyInput struct {
	Nonce             string
	UserQuery         string
	TaskInstruction   string
	OutputExample     string
	Schema            string
	SkillsContext     string
	ExtraCapabilities string
	SessionEvidence   string
	ReactiveData      string
	InjectedMemory    string
}

type LoopPromptAssemblyResult struct {
	Prompt   string
	Sections []*PromptSectionObservation
}

type loopPromptBaseMaterialProvider interface {
	GetLoopPromptBaseMaterials(tools []*aitool.Tool, nonce string) (*LoopPromptBaseMaterials, error)
}

type loopPromptAssembler interface {
	AssembleLoopPrompt(tools []*aitool.Tool, input *LoopPromptAssemblyInput) (*LoopPromptAssemblyResult, error)
}

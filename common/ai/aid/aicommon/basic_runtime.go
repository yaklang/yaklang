package aicommon

import "github.com/yaklang/yaklang/common/ai/aid/aitool"

type AIInvokeRuntime interface {
	ExecuteToolRequiredAndCall(name string) (*aitool.ToolResult, bool, error)
	AskForClarification(question string, payloads []string) string
	DirectlyAnswer(query string, tools []*aitool.Tool) (string, error)
	AddToTimeline(entry, content string)
	GetConfig() AICallerConfigIf
}

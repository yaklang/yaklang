package aid

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type AIAssistantResult struct {
	Param aitool.InvokeParams
}

type AIAssistant struct {
	Callback func(context.Context, *Config) (*AIAssistantResult, error)
}

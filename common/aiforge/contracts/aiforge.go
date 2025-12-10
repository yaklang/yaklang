package contracts

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type LiteForge interface {
	SimpleExecute(ctx context.Context, input string, aitoolOptions []aitool.ToolOption) (aitool.InvokeParams, error)
	SimpleExecuteWithOptions(ctx context.Context, input string, aitoolOptions []aitool.ToolOption, options ...aicommon.ConfigOption) (aitool.InvokeParams, error)
}

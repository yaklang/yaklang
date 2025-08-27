package contracts

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type LiteForge interface {
	SimpleExecute(ctx context.Context, input string, aitoolOptions []aitool.ToolOption, opts ...aid.Option) (aitool.InvokeParams, error)
}

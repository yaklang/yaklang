package aid

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func (c *Coordinator) CreateDatabaseSchema(input string) *schema.AIAgentRuntime {
	rt := &schema.AIAgentRuntime{
		Uuid:            c.Config.Id,
		Name:            "coordinator initializing...",
		Seq:             c.Config.GetSequenceStart(),
		QuotedUserInput: codec.StrConvQuote(input),
		ForgeName:       c.Config.ForgeName,
	}
	yakit.CreateOrUpdateAIAgentRuntime(c.Config.GetDB(), rt)
	return rt
}


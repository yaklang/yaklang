package aid

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (c *Coordinator) CreateDatabaseSchema(input string) *schema.AIAgentRuntime {
	rt := &schema.AIAgentRuntime{
		PersistentSession: c.PersistentSessionId,
		Uuid:              c.Config.Id,
		Name:              "coordinator initializing...",
		Seq:               c.Config.GetSequenceStart(),
		QuotedUserInput:   codec.StrConvQuote(input),
		ForgeName:         c.Config.ForgeName,
	}
	if err := c.Config.CreateOrUpdateRuntimeRecord(rt); err != nil {
		log.Errorf("BUG: cannot create coordinator runtime record: %v", err)
	}
	return rt
}

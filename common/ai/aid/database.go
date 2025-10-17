package aid

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func (c *Coordinator) CreateDatabaseSchema(input string) *schema.AIAgentRuntime {
	rt := &schema.AIAgentRuntime{
		Uuid:            c.config.id,
		Name:            "coordinator initializing...",
		Seq:             c.config.GetSequenceStart(),
		QuotedUserInput: codec.StrConvQuote(input),
		ForgeName:       c.GetConfig().forgeName,
	}
	yakit.CreateOrUpdateAIAgentRuntime(c.config.GetDB(), rt)
	return rt
}

func (c *Config) submitAIRequestCheckpoint(t *schema.AiCheckpoint, data *aicommon.AIRequest) error {
	return c.SubmitCheckpointRequest(t, map[string]string{
		"prompt": string(data.GetPrompt()),
	})
}

type AIResponseSimple struct {
	Reason string `json:"reason"`
	Output string `json:"output"`
}

func (c *Config) submitAIResponseCheckpoint(t *schema.AiCheckpoint, data *AIResponseSimple) error {
	return c.SubmitCheckpointResponse(t, data)
}

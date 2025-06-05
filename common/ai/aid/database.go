package aid

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func (c *Coordinator) CreateDatabaseSchema(input string) *schema.AiCoordinatorRuntime {
	rt := &schema.AiCoordinatorRuntime{
		Uuid:            c.config.id,
		Name:            "coordinator initializing...",
		Seq:             c.config.GetSequenceStart(),
		QuotedUserInput: codec.StrConvQuote(input),
		ForgeName:       c.GetConfig().forgeName,
	}
	yakit.CreateOrUpdateRuntime(c.config.GetDB(), rt)
	return rt
}

func (c *Config) GetDB() *gorm.DB {
	return consts.GetGormProjectDatabase()
}

func (c *Config) createCheckpoint(typeName schema.AiCheckpointType, id int64) *schema.AiCheckpoint {
	mod := &schema.AiCheckpoint{
		CoordinatorUuid: c.id,
		Seq:             id,
		Type:            typeName,
	}
	yakit.CreateOrUpdateCheckpoint(c.GetDB(), mod)
	return mod
}

func (c *Config) createAIInteractiveCheckpoint(id int64) *schema.AiCheckpoint {
	return c.createCheckpoint(schema.AiCheckpointType_AIInteractive, id)
}

func (c *Config) createToolCallCheckpoint(id int64) *schema.AiCheckpoint {
	return c.createCheckpoint(schema.AiCheckpointType_ToolCall, id)
}

func (c *Config) createReviewCheckpoint(id int64) *schema.AiCheckpoint {
	return c.createCheckpoint(schema.AiCheckpointType_Review, id)
}

func (c *Config) submitCheckpointRequest(t *schema.AiCheckpoint, req any) error {
	t.RequestQuotedJson = codec.StrConvQuote(string(utils.Jsonify(req)))
	return yakit.CreateOrUpdateCheckpoint(c.GetDB(), t)
}

func (c *Config) submitCheckpointResponse(t *schema.AiCheckpoint, rsp any) error {
	t.ResponseQuotedJson = codec.StrConvQuote(string(utils.Jsonify(rsp)))
	t.Finished = true
	return yakit.CreateOrUpdateCheckpoint(c.GetDB(), t)
}

func (c *Config) submitAIRequestCheckpoint(t *schema.AiCheckpoint, data *AIRequest) error {
	return c.submitCheckpointRequest(t, map[string]string{
		"prompt": string(data.GetPrompt()),
	})
}

func (c *Config) submitToolCallRequestCheckpoint(t *schema.AiCheckpoint, data *aitool.Tool, param map[string]any) error {
	return c.submitCheckpointRequest(t, map[string]any{
		"tool_name": data.Name,
		"param":     param,
	})
}

func (c *Config) submitToolCallResponse(t *schema.AiCheckpoint, result *aitool.ToolResult) error {
	return c.submitCheckpointResponse(t, result)
}

type AIResponseSimple struct {
	Reason string `json:"reason"`
	Output string `json:"output"`
}

func (c *Config) submitAIResponseCheckpoint(t *schema.AiCheckpoint, data *AIResponseSimple) error {
	return c.submitCheckpointResponse(t, data)
}

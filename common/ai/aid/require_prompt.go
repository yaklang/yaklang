package aid

import (
	"context"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// CreateRequireUserInteract 创建一个需要用户输入的提示工具
// 这个工具非常有意思，AI 如果调用这个工具，意味着，他觉得他受阻了，需要人的提示来帮助他继续完成任务
// 调用这个工具会强制让 AI 进入等待状态，直到用户输入提示
func (c *Config) CreateRequireUserInteract() (*aitool.Tool, error) {
	factory := aitool.NewFactory()
	err := factory.RegisterTool(
		"require-user-interact",
		aitool.WithDescription("require user input some prompt or selection to continue"),
		aitool.WithDangerousNoNeedTimelineRecorded(true),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithStringParam(
			"prompt",
			aitool.WithParam_Description("你想让用户回答什么问题？或者做出什么样的选择？"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"interactive_type",
			aitool.WithParam_Description("你想要的交互类型是什么？text 需要用户输入文本，select 需要用户选择一些选项"),
			aitool.WithParam_EnumString("text", "select"),
		),
		aitool.WithStructArrayParam(
			"options",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("选项列表，如果你设置的交互类型为select, 请设置这个参数"),
				aitool.WithParam_Required(true),
				aitool.WithParam_MinLength(0),
			},
			[]aitool.PropertyOption{},
			aitool.WithStringParam("value", aitool.WithParam_Description("选项的值")),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			basicPrompt := params.GetString("prompt")
			if basicPrompt == "" {
				basicPrompt = "AI Need Helps, Please Input Your Advice:"
			}
			switch strings.ToLower(params.GetString("interactive_type")) {
			case "text":
				return c.RequireUserPrompt(basicPrompt)
			case "select":
				var opts []*RequireInteractiveRequestOption
				for idx, opt := range params.GetObjectArray("options") {
					opts = append(opts, &RequireInteractiveRequestOption{
						Index:  idx,
						Prompt: opt.GetString("value"),
					})
				}
				return c.RequireUserPrompt(basicPrompt, opts...)
			default:
				return c.RequireUserPrompt(basicPrompt)
			}
		}),
	)
	if err != nil {
		return nil, err
	}
	return factory.Tools()[0], nil
}

type RequireInteractiveRequestOption struct {
	Index       int    `json:"index"`
	PromptTitle string `json:"prompt_title"`
	Prompt      string `json:"prompt"`
}

type RequireInteractiveRequest struct {
	Id      string                             `json:"id"`
	Prompt  string                             `json:"prompt"`
	Options []*RequireInteractiveRequestOption `json:"options"`
}

func (c *Config) RequireUserPromptWithEndpointResult(prompt string, opts ...*RequireInteractiveRequestOption) (aitool.InvokeParams, *aicommon.Endpoint, error) {
	return c.RequireUserPromptWithEndpointResultEx(c.ctx, prompt, opts...)
}

func (c *Config) RequireUserPromptWithEndpointResultEx(ctx context.Context, prompt string, opts ...*RequireInteractiveRequestOption) (aitool.InvokeParams, *aicommon.Endpoint, error) {
	ep := c.epm.CreateEndpointWithEventType(schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE)
	ep.SetDefaultSuggestionContinue()

	req := &RequireInteractiveRequest{
		Id:      ep.GetId(),
		Prompt:  prompt,
		Options: opts,
	}
	c.EmitRequireUserInteractive(req, ep.GetId())
	c.doWaitAgreeWithPolicy(ctx, aicommon.AgreePolicyManual, ep)
	params := ep.GetParams()
	c.ReleaseInteractiveEvent(ep.GetId(), params)
	return params, ep, nil
}

func (c *Config) RequireUserPrompt(prompt string, opts ...*RequireInteractiveRequestOption) (aitool.InvokeParams, error) {
	params, _, err := c.RequireUserPromptWithEndpointResult(prompt, opts...)
	return params, err
}

func (c *Config) EmitRequireUserInteractive(i *RequireInteractiveRequest, id string) {
	if ep, ok := c.epm.LoadEndpoint(id); ok {
		ep.SetReviewMaterials(map[string]any{
			"id":      i.Id,
			"prompt":  i.Prompt,
			"options": i.Options,
		})
		err := c.SubmitCheckpointRequest(ep.GetCheckpoint(), i)
		if err != nil {
			log.Errorf("Failed to submit checkpoint request: %v", err)
		}
	}

	c.EmitInteractiveJSON(id, schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE, "require-user-interact", i)
}

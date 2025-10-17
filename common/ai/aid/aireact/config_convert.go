package aireact

import (
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func ConvertReActConfigToAIDConfigOptions(i *ReActConfig) []aid.Option {
	opts := make([]aid.Option, 0)
	if len(i.toolKeywords) > 0 {
		opts = append(opts, aid.WithToolKeywords(i.toolKeywords...))
	}
	opts = append(opts, aid.WithDisableToolUse(i.disableToolUse))
	switch i.reviewPolicy {
	case aicommon.AgreePolicyYOLO, aicommon.AgreePolicyAuto:
		opts = append(opts, aid.WithAgreeYOLO(true))
	case aicommon.AgreePolicyAI, aicommon.AgreePolicyAIAuto:
		opts = append(opts, aid.WithAIAgree())
	case aicommon.AgreePolicyManual:
		fallthrough
	default:
		opts = append(opts, aid.WithAgreeManual())
	}
	if len(i.aiToolManagerOption) > 0 {
		opts = append(opts, aid.WithToolManagerOptions(i.aiToolManagerOption...))
	}
	opts = append(opts, aid.WithAllowPlanUserInteract(i.enableUserInteract))
	opts = append(opts, aid.WithAllowPlanUserInteract(i.enableUserInteract))
	if i.aiTransactionAutoRetry > 0 {
		opts = append(opts, aid.WithAITransactionRetry(int(i.aiTransactionAutoRetry)))
	}
	if i.timelineContentSizeLimit > 0 {
		opts = append(opts, aid.WithTimelineContentLimit(int(i.timelineContentSizeLimit)))
	}

	return opts
}

func ConvertYPBAIStartParamsToReActConfig(i *ypb.AIStartParams) []Option {
	opts := make([]Option, 0)
	if i == nil {
		return opts
	}
	if i.DisallowRequireForUserPrompt {
		opts = append(opts, WithUserInteractive(false))
	} else {
		opts = append(opts, WithUserInteractive(true))
	}

	if i.ReviewPolicy != "" {
		opts = append(opts, WithReviewPolicy(aicommon.AgreePolicyType(i.ReviewPolicy)))
	}

	if i.ReActMaxIteration > 0 {
		opts = append(opts, WithMaxIterations(int(i.ReActMaxIteration)))
	}

	if i.GetTimelineContentSizeLimit() > 0 {
		opts = append(opts, WithTimelineContentSizeLimit(i.GetTimelineContentSizeLimit()))
	}

	if i.UserInteractLimit > 0 {
		opts = append(opts, WithUserInteractiveLimitedTimes(i.UserInteractLimit))
	}

	if i.GetDisableToolUse() {
		opts = append(opts, WithDisableToolUse())
	}
	if i.GetEnableAISearchTool() {
		opts = append(opts, WithAiToolsSearchTool())
	}
	if len(i.GetExcludeToolNames()) > 0 {
		opts = append(opts, WithDisableToolsName(i.GetExcludeToolNames()...))
	}
	if len(i.GetIncludeSuggestedToolNames()) > 0 {
		opts = append(opts, WithEnableToolsName(i.GetIncludeSuggestedToolNames()...))
	}
	if len(i.GetIncludeSuggestedToolKeywords()) > 0 {
		opts = append(opts, WithToolKeywords(i.GetIncludeSuggestedToolKeywords()...))
	}
	if i.GetAIService() != "" {
		chat, err := ai.LoadChater(i.GetAIService())
		if err != nil {
			log.Errorf("load ai service failed: %v", err)
		} else {
			opts = append(opts, WithAICallback(aicommon.AIChatToAICallbackType(chat)))
		}
	}

	// 默认开启 forge 搜索
	opts = append(opts, WithAiForgeSearchTool())

	return opts
}

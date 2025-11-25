package aicommon

import (
	"context"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (c *Config) StartHotPatchLoop(ctx context.Context) {
	c.StartHotPatchOnce.Do(func() {
		if c.HotPatchOptionChan == nil {
			return
		}
		validator := make(chan struct{})
		go func() {
			for {
				select {
				case <-validator:
					log.Infof("hotpatch loop for config %s started", c.Id)
				case <-ctx.Done():
					return
				case hotPatchOption := <-c.HotPatchOptionChan.OutputChannel():
					if hotPatchOption == nil {
						log.Errorf("hotpatch option is nil, will return")
						return
					}
					err := hotPatchOption(c)
					if err != nil {
						log.Errorf("hotpatch option err: %v", err)
					} else {
						c.HotPatchBroadcaster.Submit(hotPatchOption)
					}
					c.EmitCurrentConfigInfo()
				}
			}
		}()
		validator <- struct{}{}
	})
}

func (c *Config) SimpleInfoMap() map[string]interface{} {
	return map[string]interface{}{
		"ID":                          c.Id,
		"AllowPlanUserInteract":       c.AllowPlanUserInteract,
		"PlanUserInteractMaxCount":    c.PlanUserInteractMaxCount,
		"PersistentMemory":            c.PersistentMemory,
		"TimelineRecordLimit":         0,
		"TimelineContentSizeLimit":    c.TimelineContentSizeLimit,
		"TimelineTotalContentLimit":   c.TimelineTotalContentLimit,
		"Keywords":                    c.Keywords,
		"DebugPrompt":                 c.DebugPrompt,
		"DebugEvent":                  c.DebugEvent,
		"AllowRequireForUserInteract": c.AllowRequireForUserInteract,
		"AgreePolicy":                 c.AgreePolicy,
		"AgreeInterval":               c.AgreeInterval,
		"AgreeAIScoreLow":             c.AgreeAIScoreLow,
		"AgreeAIScoreMiddle":          c.AgreeAIScoreMiddle,
		"InputConsumption":            c.InputConsumption,
		"OutputConsumption":           c.OutputConsumption,
		"AICallTokenLimit":            c.AiCallTokenLimit,
		"AIAutoRetry":                 c.AiAutoRetry,
		"AIAutoTransactionRetry":      c.AiTransactionAutoRetry,
		"GenerateReport":              c.GenerateReport,
		"ForgeName":                   c.ForgeName,
	}
}

var (
	HotPatchType_AllowRequireForUserInteract = "AllowRequireForUserInteract"
	HotPatchType_AgreePolicy                 = "AgreePolicy"
	HotPatchType_AIService                   = "AIService"
	HotPatchType_RiskControlScore            = "RiskControlScore"
)

func ProcessHotPatchMessage(e *ypb.AIInputEvent) []ConfigOption {
	if !e.IsConfigHotpatch {
		return nil
	}

	hotPatchParams := e.Params
	aiOption := make([]ConfigOption, 0)

	if e.HotpatchType == HotPatchType_AgreePolicy {
		switch hotPatchParams.GetReviewPolicy() {
		case "yolo":
			aiOption = append(aiOption, WithAgreeYOLO())
		case "ai":
			aiOption = append(aiOption, WithAIAgree())
		case "manual":
			aiOption = append(aiOption, WithAgreeManual())
		}
	}

	if e.HotpatchType == HotPatchType_RiskControlScore {
		aiOption = append(aiOption, WithAgreeAIRiskCtrlScore(hotPatchParams.GetAIReviewRiskControlScore()))
	}

	if e.HotpatchType == HotPatchType_AllowRequireForUserInteract {
		aiOption = append(aiOption, WithAllowRequireForUserInteract(!hotPatchParams.GetDisallowRequireForUserPrompt()))
	}

	if e.HotpatchType == HotPatchType_AIService {
		chat, err := ai.LoadChater(hotPatchParams.GetAIService())
		if err != nil {
			log.Errorf("load ai service failed: %v", err)
		} else {
			aiOption = append(aiOption, WithAICallback(AIChatToAICallbackType(chat)))
			aiOption = append(aiOption, WithAIServiceName(hotPatchParams.GetAIService()))
		}
	}
	return aiOption
}

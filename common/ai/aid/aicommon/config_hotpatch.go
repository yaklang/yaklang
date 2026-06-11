package aicommon

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
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
					//log.Infof("hotpatch loop for config %s started", c.Id)
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
		select {
		case validator <- struct{}{}:
		case <-ctx.Done():
		}
	})
}

func (c *Config) SimpleInfoMap() map[string]interface{} {
	input, output, _ := c.GetConsumptionConfig()
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
		"InputConsumption":            input,
		"OutputConsumption":           output,
		"CacheHitToken":               c.GetCacheHitToken(),
		"AICallTokenLimit":            c.AiCallTokenLimit,
		"AIAutoRetry":                 c.AiAutoRetry,
		"AIAutoTransactionRetry":      c.AiTransactionAutoRetry,
		"GenerateReport":              c.GenerateReport,
		"ForgeName":                   c.ForgeName,
		"EnablePlan":            c.GetEnablePlanAndExec(),
		"SyncPerceptionTrigger": c.GetSyncPerceptionTrigger(),
		"EnabledCapabilities":   c.GetEnabledCapabilities(),
	}
}

var (
	HotPatchType_AllowRequireForUserInteract = "AllowRequireForUserInteract"
	HotPatchType_AgreePolicy                 = "AgreePolicy"
	HotPatchType_AIService                   = "AIService"
	HotPatchType_ModelName                   = "ModelName"
	HotPatchType_RiskControlScore            = "RiskControlScore"
	HotPatchType_EnablePlan                  = "EnablePlan"
	HotPatchType_AllowPlanUserInteract       = "AllowPlanUserInteract"
	HotPatchType_SyncPerceptionTrigger       = "SyncPerceptionTrigger"

	hotPatchPromoteIntelligentConfig = func(serviceName, modelName string) error {
		mgr := aiconfig.GetGlobalManager()
		return mgr.PromoteFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, serviceName, modelName)
	}
	hotPatchGetIntelligentCallback = func(serviceName, modelName string) (AICallbackType, error) {
		return GetAIModelCallbackByTierAndProviderAndModel(consts.TierIntelligent, serviceName, modelName)
	}
	hotPatchLoadChater = func(serviceName string, defaultOpts ...aispec.AIConfigOption) (aispec.GeneralChatter, error) {
		return ai.LoadChater(serviceName, defaultOpts...)
	}
)

func (c *Config) ProcessHotPatchMessage(e *ypb.AIInputEvent) []ConfigOption {
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
		serviceName := hotPatchParams.GetAIService()
		modelName := hotPatchParams.GetAIModelName()

		if serviceName == "" {
			log.Errorf("hotpatch AIService is empty")
			return aiOption
		}

		if err := hotPatchPromoteIntelligentConfig(serviceName, modelName); err != nil {
			log.Warnf("failed to promote intelligent tier model by service=%s model=%s: %v", serviceName, modelName, err)
		}

		if cb, err := hotPatchGetIntelligentCallback(serviceName, modelName); err == nil {
			aiOption = append(aiOption, WithQualityPriorityAICallback(cb))
		} else {
			log.Warnf("load callback from tiered config failed by service=%s model=%s: %v", serviceName, modelName, err)

			defaultOpts := make([]aispec.AIConfigOption, 0, 1)
			if modelName != "" {
				defaultOpts = append(defaultOpts, aispec.WithModel(modelName))
			}
			chat, loadErr := hotPatchLoadChater(serviceName, defaultOpts...)
			if loadErr != nil {
				log.Errorf("load ai service failed: %v", loadErr)
			} else {
				aiOption = append(aiOption, WithQualityPriorityAICallback(AIChatToAICallbackType(chat)))
			}
		}
		aiOption = append(aiOption, WithAIChatInfo(serviceName, modelName))
	}

	if e.HotpatchType == HotPatchType_EnablePlan {
		aiOption = append(aiOption, WithEnablePlanAndExec(hotPatchParams.GetEnablePlan()))
	}

	if e.HotpatchType == HotPatchType_AllowPlanUserInteract {
		aiOption = append(aiOption, WithAllowPlanUserInteract(hotPatchParams.GetAllowPlanUserInteract()))
	}

	if e.HotpatchType == HotPatchType_SyncPerceptionTrigger {
		aiOption = append(aiOption, WithSyncPerceptionTrigger(hotPatchParams.GetSyncPerceptionTrigger()))
	}

	if e.HotpatchType == HotPatchType_EnabledCapabilities {
		incoming := ParseEnabledCapabilitiesFromProto(hotPatchParams)
		if len(incoming) > 0 {
			merged := append(c.GetEnabledCapabilities(), incoming...)
			aiOption = append(aiOption, WithEnabledCapabilities(merged...))
		}
	}

	if e.HotpatchType == HotPatchType_DisabledCapabilities {
		incoming := ParseEnabledCapabilitiesFromProto(hotPatchParams)
		if len(incoming) > 0 {
			aiOption = append(aiOption, WithDisabledCapabilities(incoming...))
		}
	}

	if e.HotpatchType == HotPatchType_ModelName {
		serviceName := c.AiServerName
		modelName := hotPatchParams.GetAIModelName()
		if err := hotPatchPromoteIntelligentConfig(serviceName, modelName); err != nil {
			log.Warnf("failed to promote intelligent tier model by service=%s model=%s: %v", serviceName, modelName, err)
		}

		if cb, err := hotPatchGetIntelligentCallback(serviceName, modelName); err == nil {
			aiOption = append(aiOption, WithQualityPriorityAICallback(cb))
		} else {
			log.Warnf("load callback from tiered config failed by service=%s model=%s: %v", serviceName, modelName, err)

			defaultOpts := make([]aispec.AIConfigOption, 0, 1)
			if modelName != "" {
				defaultOpts = append(defaultOpts, aispec.WithModel(modelName))
			}
			chat, loadErr := hotPatchLoadChater(serviceName, defaultOpts...)
			if loadErr != nil {
				log.Errorf("load ai service failed: %v", loadErr)
			} else {
				aiOption = append(aiOption, WithQualityPriorityAICallback(AIChatToAICallbackType(chat)))
			}
		}
		aiOption = append(aiOption, WithAIChatInfo(serviceName, modelName))
		log.Warnf("HotPatch ModelName is deprecated, " +
			"model info is now auto-detected from the actual AI gateway call")
	}

	return aiOption
}

// mergeHotpatchSessionStartParams overlays hotpatch Params onto cached session start_params.
// Bool fields use HotpatchType to distinguish explicit false from proto3 zero values.
func mergeHotpatchSessionStartParams(base *ypb.AIStartParams, e *ypb.AIInputEvent) (*ypb.AIStartParams, bool) {
	if e == nil || e.Params == nil {
		return base, false
	}
	hotpatchType := strings.TrimSpace(e.GetHotpatchType())
	if hotpatchType == "" {
		return base, false
	}

	var next *ypb.AIStartParams
	if base == nil {
		next = &ypb.AIStartParams{}
	} else {
		next = proto.Clone(base).(*ypb.AIStartParams)
	}
	p := e.Params

	switch hotpatchType {
	case HotPatchType_EnablePlan:
		next.EnablePlan = p.GetEnablePlan()
	case HotPatchType_SyncPerceptionTrigger:
		next.SyncPerceptionTrigger = p.GetSyncPerceptionTrigger()
	case HotPatchType_EnabledCapabilities:
		if len(p.GetEnabledCapabilities()) == 0 {
			return base, false
		}
		next.EnabledCapabilities = MergeEnabledCapabilitiesHotpatch(base, p)
	case HotPatchType_DisabledCapabilities:
		if len(p.GetEnabledCapabilities()) == 0 {
			return base, false
		}
		next.EnabledCapabilities = SubtractEnabledCapabilitiesHotpatch(base, p)
	case HotPatchType_AllowPlanUserInteract:
		next.AllowPlanUserInteract = p.GetAllowPlanUserInteract()
	case HotPatchType_AllowRequireForUserInteract:
		next.DisallowRequireForUserPrompt = p.GetDisallowRequireForUserPrompt()
	case HotPatchType_AgreePolicy:
		if p.GetReviewPolicy() == "" {
			return base, false
		}
		next.ReviewPolicy = p.GetReviewPolicy()
	case HotPatchType_RiskControlScore:
		next.AIReviewRiskControlScore = p.GetAIReviewRiskControlScore()
	case HotPatchType_AIService:
		if p.GetAIService() == "" {
			return base, false
		}
		next.AIService = p.GetAIService()
		if p.GetAIModelName() != "" {
			next.AIModelName = p.GetAIModelName()
		}
	case HotPatchType_ModelName:
		if p.GetAIModelName() == "" {
			return base, false
		}
		next.AIModelName = p.GetAIModelName()
	default:
		return base, false
	}
	return next, true
}

func (c *Config) PersistSessionStartParamsFromHotpatch(e *ypb.AIInputEvent) {
	if c == nil || e == nil || !e.IsConfigHotpatch {
		return
	}
	sessionID := strings.TrimSpace(c.PersistentSessionId)
	if sessionID == "" || c.GetDB() == nil {
		return
	}

	cached, err := yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	if err != nil {
		log.Warnf("load ai session start params failed for %s: %v", sessionID, err)
	}
	next, changed := mergeHotpatchSessionStartParams(cached, e)
	if !changed {
		return
	}
	if _, err := yakit.CreateOrUpdateAISessionMetaStartParams(c.GetDB(), sessionID, next); err != nil {
		log.Warnf("persist ai session start params from hotpatch failed for %s: %v", sessionID, err)
	}
}

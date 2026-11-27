package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
		c.hotPatchLoopWG.Add(1)
		go func() {
			defer c.hotPatchLoopWG.Done()
			for {
				select {
				case <-validator:
					//log.Infof("hotpatch loop for config %s started", c.Id)
				case <-ctx.Done():
					return
				case hotPatchOption, ok := <-c.HotPatchOptionChan.OutputChannel():
					if !ok {
						return
					}
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

// WaitHotPatchLoopStopped waits until the hot-patch consumer has completely
// exited. Session lifecycle owners use it before deleting persistent session
// data, because a final hot-patch may still emit and persist config events while
// the main input loop is already stopping.
func (c *Config) WaitHotPatchLoopStopped() {
	if c == nil {
		return
	}
	c.hotPatchLoopWG.Wait()
}

func (c *Config) SimpleInfoMap() map[string]interface{} {
	input, output, _ := c.GetConsumptionConfig()
	goalTime := c.GetGoalTimeState()
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
		"EnablePlan":                  c.GetEnablePlanAndExec(),
		"SyncPerceptionTrigger":       c.GetSyncPerceptionTrigger(),
		"EnabledCapabilities":         c.GetEnabledCapabilities(),
		"EnableMultiAgent":            c.GetPreferDispatchSubReactAgents(),
		"EnableGoalMode":              c.GetEnableGoalMode(),
		"GoalMinIterations":           c.GetGoalMinIterations(),
		"MaxSubAgents":                c.GetMaxSubAgents(),
		"GoalDurationSeconds":         goalTime.DurationSeconds,
		"GoalAcceptanceCriteria":      c.GetGoalAcceptanceCriteria(),
		"GoalTimeRevision":            goalTime.Revision,
		"GoalTimeStarted":             goalTime.Started,
		"GoalTimeNeverEnding":         goalTime.NeverEnding,
		"GoalDeadlineUnix":            goalTime.DeadlineUnix,
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
	HotPatchType_ExecutionStrategy           = "ExecutionStrategy"
	HotPatchType_GoalTime                    = "GoalTime"
	HotPatchType_RuntimeContextSizeLimit     = "RuntimeContextSizeLimit"

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

// runtimeContextSizeLimitFromHotpatch maps the legacy hotpatch onto the config.
// Keep non-positive values compatible with the original server default.
func runtimeContextSizeLimitFromHotpatch(raw int64) int {
	if raw <= 0 {
		return DefaultTimelineContentSizeLimit
	}
	return int(raw)
}

func (c *Config) ProcessHotPatchMessage(e *ypb.AIInputEvent) []ConfigOption {
	if e == nil || !e.IsConfigHotpatch {
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
			aiOption = append(aiOption, wrapHotpatchOptionForTask(e.GetTaskId(), WithHotpatchEnabledCapabilities(incoming...)))
		}
	}

	if e.HotpatchType == HotPatchType_DisabledCapabilities {
		incoming := ParseEnabledCapabilitiesFromProto(hotPatchParams)
		if len(incoming) > 0 {
			aiOption = append(aiOption, wrapHotpatchOptionForTask(e.GetTaskId(), WithHotpatchDisabledCapabilities(incoming...)))
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

	if e.HotpatchType == HotPatchType_RuntimeContextSizeLimit && hotPatchParams.GetModelContextKey() == "" {
		aiOption = append(aiOption, WithTimelineContentLimit(runtimeContextSizeLimitFromHotpatch(hotPatchParams.GetRuntimeContextSizeLimit())))
	}
	if e.HotpatchType == HotPatchType_RuntimeContextSizeLimit && hotPatchParams.GetModelContextKey() != "" {
		raw := hotPatchParams.GetRuntimeContextSizeLimit()
		aiOption = append(aiOption, func(config *Config) error {
			provider, model, key := config.PrimaryModelContext()
			// Routing comes from the backend's quality configuration. The opaque
			// key binds this edit to the configuration the user actually saw.
			if key == "" || hotPatchParams.GetModelContextKey() != key {
				status := config.ModelContextStatus(0)
				status.Error = "模型配置已变化，请刷新上下文设置后重试"
				config.emitModelContextStatus(status)
				return fmt.Errorf("%s", status.Error)
			}
			if err := yakit.ValidateAIModelContextThreshold(raw); err != nil {
				status := config.ModelContextStatus(0)
				status.Error = err.Error()
				config.emitModelContextStatus(status)
				return err
			}
			previous := config.ModelContextStatusFor(provider, model, 0, key).ThresholdTokens
			if err := yakit.SetAIModelContextSettings(consts.GetGormProfileDatabase(), provider, model, raw,
				hotPatchParams.DisableContextAutoAdjust, key); err != nil {
				status := config.ModelContextStatusFor(provider, model, 0, key)
				status.Error = err.Error()
				config.emitModelContextStatus(status)
				return err
			}

			config.resetModelContextLimit(provider, model, key)
			status := config.ModelContextStatusFor(provider, model, 0, key)
			if timeline := config.GetTimeline(); timeline != nil {
				timeline.SetTimelineContentLimit(status.ThresholdTokens)
			}
			status.Change = newModelContextChange("manual", previous, status.ThresholdTokens)
			config.emitModelContextStatus(status)
			return nil
		})
	}

	if e.HotpatchType == HotPatchType_GoalTime {
		if patch := e.GetGoalTimePatch(); patch != nil {
			aiOption = append(aiOption, WithGoalTimePatch(patch))
		}
		return c.withAgentPolicySnapshotEmit(aiOption)
	}

	if e.HotpatchType == HotPatchType_ExecutionStrategy {
		strategy := hotPatchParams.GetStrategy()
		if strategy != nil {
			aiOption = append(aiOption, WithExecutionStrategy(strategy))
		}
		if patch := e.GetGoalTimePatch(); patch != nil {
			aiOption = append(aiOption, WithGoalTimePatch(patch))
		}
	}
	if hotpatchAffectsAgentPolicy(e.HotpatchType) {
		return c.withAgentPolicySnapshotEmit(aiOption)
	}
	return aiOption
}

func validateExecutionStrategy(strategy *ypb.AIExecutionStrategy) error {
	if strategy == nil {
		return nil
	}
	if strategy.GetGoalMinIterations() < 0 {
		return fmt.Errorf("goal min iterations must be >= 0")
	}
	if strategy.GetMaxSubAgents() < 0 {
		return fmt.Errorf("max sub agents must be >= 0")
	}
	return validateGoalDurationSeconds(strategy.GetGoalDurationSeconds())
}

type executionPolicyDraft struct {
	enableDispatchSubReactAgents bool
	preferDispatchSubReactAgents bool
	maxSubAgents                 int64
	enableGoalMode               bool
	goalMinIterations            int64
	goalAcceptanceCriteria       string
	goalDurationSeconds          int64     // declared config; GoalTimePatch only reads it
	goalDeadline                 time.Time // runtime window; the only field patches write
	goalTimeRevision             uint64
}

func (c *Config) currentExecutionPolicyDraft() executionPolicyDraft {
	return executionPolicyDraft{
		enableDispatchSubReactAgents: c.EnableDispatchSubReactAgents,
		preferDispatchSubReactAgents: c.PreferDispatchSubReactAgents,
		maxSubAgents:                 c.MaxSubAgents,
		enableGoalMode:               c.EnableGoalMode,
		goalMinIterations:            c.GoalMinIterations,
		goalAcceptanceCriteria:       c.GoalAcceptanceCriteria,
		goalDurationSeconds:          c.GoalDurationSeconds,
		goalDeadline:                 c.GoalDeadline,
		goalTimeRevision:             c.goalTimeRevision,
	}
}

func (d *executionPolicyDraft) applyExecutionStrategy(strategy *ypb.AIExecutionStrategy, now time.Time) error {
	if err := validateExecutionStrategy(strategy); err != nil {
		return err
	}
	// Strategy is the declarative source: it replaces the declared length and
	// materializes a window for it. GoalTimePatch never writes the declared
	// value, so a runtime command cannot rewrite the user's configuration.
	if applyDeclaredGoalDuration(&d.goalDurationSeconds, &d.goalDeadline, strategy.GetGoalDurationSeconds(), now) {
		d.goalTimeRevision++
	}
	d.enableDispatchSubReactAgents = strategy.GetEnableMultiAgent()
	d.preferDispatchSubReactAgents = strategy.GetEnableMultiAgent()
	d.maxSubAgents = normalizeMaxSubAgentsLocked(strategy.GetMaxSubAgents())
	d.enableGoalMode = strategy.GetEnableGoalMode()
	d.goalMinIterations = strategy.GetGoalMinIterations()
	d.goalAcceptanceCriteria = strings.TrimSpace(strategy.GetGoalAcceptanceCriteria())
	return nil
}

func (d *executionPolicyDraft) applyGoalTimePatch(patch *ypb.AIGoalTimePatch, now time.Time) error {
	if patch == nil {
		return nil
	}
	var (
		changed bool
		err     error
	)
	switch patch.GetAction() {
	case ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_EXTEND:
		changed, err = d.extendGoalTime(patch.GetExtendSeconds(), now)
	case ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE:
		changed, err = d.setGoalDeadline(patch.GetDeadlineUnix())
	case ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_DISABLE:
		changed = d.disableGoalTime()
	case ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_NEVER_ENDING:
		changed = d.disableGoalDeadlineExpiry(now)
	case ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DURATION:
		changed, err = d.replaceGoalDuration(patch.GetDurationSeconds(), now)
	default:
		return fmt.Errorf("unsupported goal time action: %s", patch.GetAction())
	}
	if err != nil {
		return err
	}
	if changed {
		d.goalTimeRevision++
	}
	return nil
}

func (d *executionPolicyDraft) extendGoalTime(delta int64, now time.Time) (bool, error) {
	if delta == 0 {
		return false, fmt.Errorf("goal time extension must not be zero")
	}
	switch {
	case isGoalNeverEndingDeadline(d.goalDeadline):
		return false, nil
	case d.goalDeadline.IsZero():
		// No window is open. Read the declared length to reopen one; the
		// declared value itself is never written back.
		switch {
		case d.goalDurationSeconds == 0:
			return false, fmt.Errorf("goal time window is disabled")
		case d.goalDurationSeconds == -1:
			d.goalDeadline = computeGoalDeadline(now, -1)
		default:
			nextDuration := d.goalDurationSeconds + delta
			if nextDuration <= 0 {
				return false, fmt.Errorf("goal duration must remain positive")
			}
			d.goalDeadline = computeGoalDeadline(now, nextDuration)
		}
	default:
		base := d.goalDeadline
		if base.Before(now) {
			base = now
		}
		d.goalDeadline = base.Add(time.Duration(delta) * time.Second)
		if d.goalDeadline.Before(now) {
			d.goalDeadline = now
		}
	}
	return true, nil
}

func (d *executionPolicyDraft) setGoalDeadline(deadlineUnix int64) (bool, error) {
	if deadlineUnix <= 0 {
		return false, fmt.Errorf("goal deadline unix timestamp must be positive")
	}
	d.goalDeadline = time.Unix(deadlineUnix, 0)
	return true, nil
}

func (d *executionPolicyDraft) disableGoalTime() bool {
	changed := !d.goalDeadline.IsZero()
	d.goalDeadline = time.Time{}
	return changed
}

func (d *executionPolicyDraft) disableGoalDeadlineExpiry(now time.Time) bool {
	d.goalDeadline = computeGoalDeadline(now, -1)
	return true
}

// replaceGoalDuration starts a fresh window of the requested length from now.
// It is the only imperative action that can move a disabled (0) or never-ending
// (-1) window back to a finite one, because SET_DEADLINE cannot express that
// intent without also rewriting the duration.
//
// Unlike the declarative ExecutionStrategy duration, this always restarts the
// window: the caller explicitly asked for "this much time from now".
func (d *executionPolicyDraft) replaceGoalDuration(seconds int64, now time.Time) (bool, error) {
	if err := validateGoalDurationSeconds(seconds); err != nil {
		return false, err
	}
	if seconds <= 0 {
		return false, fmt.Errorf("goal duration must be positive for SET_DURATION; use DISABLE or NEVER_ENDING instead")
	}
	d.goalDeadline = computeGoalDeadline(now, seconds)
	return true, nil
}

func (d executionPolicyDraft) commit(c *Config) {
	c.EnableDispatchSubReactAgents = d.enableDispatchSubReactAgents
	c.PreferDispatchSubReactAgents = d.preferDispatchSubReactAgents
	c.MaxSubAgents = d.maxSubAgents
	c.EnableGoalMode = d.enableGoalMode
	c.GoalMinIterations = d.goalMinIterations
	c.GoalAcceptanceCriteria = d.goalAcceptanceCriteria
	c.GoalDurationSeconds = d.goalDurationSeconds
	c.GoalDeadline = d.goalDeadline
	c.goalTimeRevision = d.goalTimeRevision
}

// DispatchHotPatch applies agent policy commands before the event loop advances,
// so a following sync request observes the committed state. Other hotpatches
// retain the existing queue and broadcaster behavior.
func (c *Config) DispatchHotPatch(event *ypb.AIInputEvent) error {
	if c == nil || event == nil || !event.GetIsConfigHotpatch() {
		return nil
	}

	if !hotpatchAffectsAgentPolicy(event.GetHotpatchType()) {
		options := c.ProcessHotPatchMessage(event)
		c.PersistSessionStartParamsFromHotpatch(event)
		for _, option := range options {
			c.HotPatchOptionChan.SafeFeed(option)
		}
		return nil
	}
	c.agentPolicyHotpatchMu.Lock()
	defer c.agentPolicyHotpatchMu.Unlock()
	if event.GetHotpatchType() == HotPatchType_ExecutionStrategy {
		if strategy := event.Params.GetStrategy(); strategy != nil {
			if err := validateExecutionStrategy(strategy); err != nil {
				return err
			}
		}
	}
	options := c.ProcessHotPatchMessage(event)
	if len(options) == 0 {
		return nil
	}
	applyErr := c.applyLocalHotPatchOptions(options)
	c.PersistSessionStartParamsFromHotpatch(event)
	c.EmitCurrentConfigInfo()
	return applyErr
}

func (c *Config) applyLocalHotPatchOptions(options []ConfigOption) error {
	var firstErr error
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(c); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func hotpatchAffectsAgentPolicy(hotpatchType string) bool {
	switch hotpatchType {
	case HotPatchType_ExecutionStrategy,
		HotPatchType_GoalTime,
		HotPatchType_EnablePlan,
		HotPatchType_AllowPlanUserInteract:
		return true
	default:
		return false
	}
}

func (c *Config) withAgentPolicySnapshotEmit(options []ConfigOption) []ConfigOption {
	if len(options) == 0 {
		return options
	}
	return append(options, c.agentPolicySnapshotEmitOption())
}

// WithExecutionStrategy replaces the declared execution policy.
func WithExecutionStrategy(strategy *ypb.AIExecutionStrategy) ConfigOption {
	return func(c *Config) error {
		return c.applyExecutionStrategy(strategy)
	}
}

func (c *Config) applyExecutionStrategy(strategy *ypb.AIExecutionStrategy) error {
	if c == nil {
		return fmt.Errorf("execution strategy config is nil")
	}
	if strategy == nil {
		return fmt.Errorf("execution strategy is nil")
	}
	c.m.Lock()
	defer c.m.Unlock()
	next := c.currentExecutionPolicyDraft()
	if err := next.applyExecutionStrategy(strategy, time.Now()); err != nil {
		return err
	}
	next.commit(c)
	return nil
}

// WithGoalTimePatch applies an imperative runtime goal-time mutation.
func WithGoalTimePatch(patch *ypb.AIGoalTimePatch) ConfigOption {
	return func(c *Config) error {
		return c.applyGoalTimePatch(patch)
	}
}

func (c *Config) applyGoalTimePatch(patch *ypb.AIGoalTimePatch) error {
	if c == nil {
		return fmt.Errorf("goal time config is nil")
	}
	if patch == nil {
		return fmt.Errorf("goal time patch is nil")
	}
	c.m.Lock()
	defer c.m.Unlock()
	next := c.currentExecutionPolicyDraft()
	if err := next.applyGoalTimePatch(patch, time.Now()); err != nil {
		return err
	}
	next.commit(c)
	return nil
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
	case HotPatchType_ExecutionStrategy:
		incoming := p.GetStrategy()
		if incoming == nil {
			return base, false
		}
		next.Strategy = proto.Clone(incoming).(*ypb.AIExecutionStrategy)
	case HotPatchType_RuntimeContextSizeLimit:
		if p.GetModelContextKey() == "" {
			// Persist the raw value, including 0. A positive value is an explicit
			// threshold; <=0 means the next start keeps the server default instead
			// of replaying the previous explicit limit.
			limit := p.GetRuntimeContextSizeLimit()
			if limit < 0 {
				limit = 0
			}
			next.RuntimeContextSizeLimit = limit
			return next, true
		}
		limit := p.GetRuntimeContextSizeLimit()
		if yakit.ValidateAIModelContextThreshold(limit) != nil || (limit == 0 && p.DisableContextAutoAdjust == nil) {
			return base, false
		}
		// The caller validates the current model identity and upper bound.
		// On resume, read its saved setting instead of replaying a session limit.
		next.RuntimeContextSizeLimit = 0

	default:
		return base, false
	}
	return next, true
}

func (c *Config) SetHotpatchCurrentTaskIdResolver(resolver func() string) {
	if c == nil {
		return
	}
	if c.m == nil {
		c.m = new(sync.Mutex)
	}
	c.m.Lock()
	defer c.m.Unlock()
	c.hotpatchCurrentTaskIdResolver = resolver
}

func (c *Config) resolveHotpatchCurrentTaskId() string {
	if c == nil {
		return ""
	}
	c.m.Lock()
	resolver := c.hotpatchCurrentTaskIdResolver
	c.m.Unlock()
	if resolver == nil {
		return ""
	}
	return strings.TrimSpace(resolver())
}

func isTaskScopedCapabilityHotpatch(e *ypb.AIInputEvent) bool {
	if e == nil {
		return false
	}
	if strings.TrimSpace(e.GetTaskId()) == "" {
		return false
	}
	switch strings.TrimSpace(e.GetHotpatchType()) {
	case HotPatchType_EnabledCapabilities, HotPatchType_DisabledCapabilities:
		return true
	default:
		return false
	}
}

func wrapHotpatchOptionForTask(targetTaskId string, opt ConfigOption) ConfigOption {
	targetTaskId = strings.TrimSpace(targetTaskId)
	if opt == nil {
		return func(*Config) error { return nil }
	}
	if targetTaskId == "" {
		return opt
	}
	return func(c *Config) error {
		if c == nil {
			return nil
		}
		currentTaskId := c.resolveHotpatchCurrentTaskId()
		if currentTaskId != targetTaskId {
			if c.DebugEvent {
				log.Infof("skip hotpatch for task %q on config %s (current task: %q)",
					targetTaskId, c.Id, currentTaskId)
			}
			return nil
		}
		return opt(c)
	}
}

func (c *Config) PersistSessionStartParamsFromHotpatch(e *ypb.AIInputEvent) {
	if c == nil || e == nil || !e.IsConfigHotpatch {
		return
	}
	if isTaskScopedCapabilityHotpatch(e) {
		return
	}
	if e.HotpatchType == HotPatchType_RuntimeContextSizeLimit && e.Params.GetModelContextKey() != "" {
		provider, model, key := c.PrimaryModelContext()
		if key == "" || e.Params.GetModelContextKey() != key {
			return
		}
		if yakit.ValidateAIModelContextThreshold(e.Params.GetRuntimeContextSizeLimit()) != nil {
			return
		}
		value := yakit.GetAIModelContext(consts.GetGormProfileDatabase(), provider, model, key)
		if e.Params.GetRuntimeContextSizeLimit() > yakit.AIModelContextMaximumThreshold(value) {
			return
		}
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

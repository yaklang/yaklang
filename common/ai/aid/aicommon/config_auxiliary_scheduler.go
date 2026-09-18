package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

// ScheduleAuxiliaryTask is the Config-owned entry point for auxiliary AI work.
// In single-model mode the registry decides whether the task is skipped,
// executed with lightweight request parameters, or passed through unchanged.
// Outside single-model mode every task preserves its existing behavior.
func (c *Config) ScheduleAuxiliaryTask(
	ctx context.Context,
	name string,
	promptBuilder func() string,
	onResult func(*Action),
	opts ...AuxiliaryTaskOption,
) {
	if c == nil || promptBuilder == nil {
		return
	}

	decision := c.ResolveAuxiliaryTask(name)
	if !decision.ShouldRun() {
		return
	}
	spec := &AuxiliaryTaskSpec{Emitter: c.Emitter}
	for _, opt := range opts {
		if opt != nil {
			opt(spec)
		}
	}
	prompt := promptBuilder()
	if prompt == "" {
		return
	}
	if len(decision.RequestOpts) > 0 {
		spec.Opts = append([]GeneralKVConfigOption(nil), spec.Opts...)
		spec.Opts = append(spec.Opts, WithGeneralConfigExtraRequestOpts(decision.RequestOpts...))
	}

	if utils.IsNil(ctx) {
		ctx = c.GetContext()
	}
	if utils.IsNil(ctx) {
		ctx = context.Background()
	}

	invokeOpts := []any{
		&LiteForgeInvokeRequest{
			Context:          ctx,
			ActionName:       name,
			OutputActionName: spec.OutputActionName,
			OutputSchema:     spec.OutputSchema,
			Outputs:          spec.Outputs,
			Options:          spec.Opts,
			Emitter:          spec.Emitter,
		},
		WithAgreeYOLO(),
		WithAITransactionAutoRetry(c.GetAITransactionAutoRetryCount()),
		WithPersistentSessionId(c.PersistentSessionId),
	}
	if retryWait := c.GetAIRetryWaitFunc(); retryWait != nil {
		invokeOpts = append(invokeOpts, WithAIRetryWaitFunc(retryWait))
	}
	if userUsageCallback := c.GetUserUsageCallback(); userUsageCallback != nil {
		invokeOpts = append(invokeOpts, WithUserUsageCallback(userUsageCallback))
	}
	if c.EventHandler != nil {
		invokeOpts = append(invokeOpts, WithEventHandler(c.EventHandler))
	}

	result, err := c.invokeSpeedPriorityLiteForge(prompt, invokeOpts...)
	if err != nil || result == nil || result.Action == nil {
		if err == nil {
			err = utils.Errorf("auxiliary task %q returned no action", name)
		}
		if spec.OnError != nil {
			spec.OnError(err)
		}
		return
	}
	if onResult != nil {
		onResult(result.Action)
	}
}

// ResolveAuxiliaryTask exposes the Config-owned policy to non-LiteForge Speed
// calls without changing their prompt or response protocol.
func (c *Config) ResolveAuxiliaryTask(name string) AuxiliaryTaskDecision {
	decision := AuxiliaryTaskDecision{Action: SingleModelPassThrough}
	if c == nil {
		return decision
	}
	action := SingleModelPassThrough
	if c.IsSingleAIModelMode() {
		action = GetSingleModelAction(name)
	}
	decision.Action = action
	if action == SingleModelLiteCall {
		decision.RequestOpts = []AIRequestOption{
			WithAIRequest_ExtraSpecOpts(aispec.WithThinkingLevel("none")),
		}
	}
	return decision
}

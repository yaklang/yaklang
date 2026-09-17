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

	action := SingleModelPassThrough
	if c.IsSingleAIModelMode() {
		action = GetSingleModelAction(name)
		if action == SingleModelSkip {
			return
		}
	}

	prompt := promptBuilder()
	if prompt == "" {
		return
	}

	spec := &AuxiliaryTaskSpec{}
	for _, opt := range opts {
		if opt != nil {
			opt(spec)
		}
	}

	if action == SingleModelLiteCall {
		spec.Opts = append(spec.Opts, WithGeneralConfigExtraRequestOpts(
			WithAIRequest_ExtraSpecOpts(aispec.WithThinkingLevel("none")),
		))
	}

	if utils.IsNil(ctx) {
		ctx = c.GetContext()
	}
	if utils.IsNil(ctx) {
		ctx = context.Background()
	}

	invokeOpts := []any{
		&LiteForgeInvokeRequest{
			Context:    ctx,
			ActionName: name,
			Outputs:    spec.Outputs,
			Options:    spec.Opts,
			Emitter:    c.Emitter,
		},
		WithAgreeYOLO(),
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

	result, err := c.InvokeLiteForge(prompt, invokeOpts...)
	if err != nil || result == nil || result.Action == nil {
		return
	}
	if onResult != nil {
		onResult(result.Action)
	}
}

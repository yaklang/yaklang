package aicommon

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
)

// The host-state unit tests cannot import aiforge from package aicommon.
// Register a scoped adapter for reducers only, and restore the prior callback.
// The real renderer, streaming callbacks and lifecycle are exercised separately
// in aid/liteforge/liteforgeapp/auxiliary_stream_test.go.
func registerTimelineTestLiteForge(t testing.TB) {
	t.Helper()
	previous := liteforgeExecuteFunc
	RegisterLiteForgeExecuteCallback(func(prompt string, opts ...any) (*ForgeResult, error) {
		var req *LiteForgeInvokeRequest
		var configOpts []ConfigOption
		for _, opt := range opts {
			switch value := opt.(type) {
			case *LiteForgeInvokeRequest:
				req = value
			case ConfigOption:
				configOpts = append(configOpts, value)
			}
		}
		if req == nil || req.ActionName != CallerLabelTimelineCompress {
			if previous != nil {
				return previous(prompt, opts...)
			}
			return nil, errors.New("no test LiteForge adapter for this task")
		}
		g := NewGeneralKVConfig(req.Options...)
		require.True(t, g.GetLiteForgeDisableTimeline(), "the snapshot is the only history source")
		// Instructions now live in the rendered prompt, not a separate option.
		// Still verify that the complete instruction and output contract arrive.
		source := promptloader.MustLoad("ai/aid/aicommon/prompts/timeline/compression.txt")
		instruction, _, found := strings.Cut(source, "{{ .Capacity }}")
		require.True(t, found, "the template must place its fixed instruction before the capacity")
		require.True(t, strings.HasPrefix(prompt, instruction), "the request must contain the entire fixed instruction")
		require.Equal(t, 1, strings.Count(prompt, strings.TrimSpace(instruction)), "the instruction must be supplied exactly once")
		require.Equal(t, timelineCompressionSchema, req.OutputSchema)
		require.Equal(t, 1, strings.Count(prompt, req.OutputSchema), "the complete output schema must be supplied exactly once")
		// NewConfig SoftBinds whatever Timeline it owns and copies that config's
		// content limit onto it. Production liteforge.Execute drops or copies a
		// shared Timeline before construction. This adapter must do the same,
		// or the helper's default 300K limit overwrites the parent halfway
		// through compression.
		configOpts = append([]ConfigOption{WithDisableAutoSkills(true)}, configOpts...)
		configOpts = append(configOpts, func(cfg *Config) error {
			if g.GetLiteForgeDisableTimeline() {
				cfg.Timeline = nil
			} else if cfg.Timeline != nil {
				cfg.Timeline = cfg.Timeline.CopyReducibleTimelineWithMemory()
			}
			return nil
		})
		cfg := NewConfig(req.Context, configOpts...)
		requestPrompt := prompt
		if req.ResponseHandler == nil {
			requestPrompt = NewGeneralKVConfig(req.Options...).GetLiteForgeStaticInstruction() + "\n" + prompt + "\n" + req.OutputSchema
		}
		request := NewAIRequest(requestPrompt, NewGeneralKVConfig(req.Options...).GetExtraRequestOpts()...)
		response, err := cfg.CallAI(request)
		if err != nil {
			return nil, err
		}
		var action *Action
		if req.ResponseHandler != nil {
			action, err = req.ResponseHandler(response)
		} else {
			action, err = ExtractValidActionFromStream(req.Context, response.GetUnboundStreamReader(false), req.OutputActionName)
		}
		if err != nil {
			return nil, err
		}
		return &ForgeResult{Action: action}, nil
	})
	t.Cleanup(func() { RegisterLiteForgeExecuteCallback(previous) })
}

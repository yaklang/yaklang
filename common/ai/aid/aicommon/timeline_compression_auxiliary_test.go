package aicommon

import (
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

// The host-state unit tests cannot import aiforge from package aicommon.
// Register a scoped adapter for reducers only, and restore the prior callback.
// The real renderer, streaming callbacks and lifecycle are exercised separately
// in aiforge/liteforge_auxiliary_stream_test.go.
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
		cfg := NewConfig(req.Context, append([]ConfigOption{WithDisableAutoSkills(true)}, configOpts...)...)
		if req.ActionName == CallerLabelTimelineCompress {
			require.True(t, NewGeneralKVConfig(req.Options...).GetLiteForgeDisableTimeline(), "the snapshot is the only history source")
		}
		request := NewAIRequest(prompt+"\n"+req.OutputSchema,
			NewGeneralKVConfig(req.Options...).GetExtraRequestOpts()...)
		response, err := cfg.CallAI(request)
		if err != nil {
			return nil, err
		}
		action, err := ExtractValidActionFromStream(req.Context, response.GetUnboundStreamReader(false), req.OutputActionName)
		if err != nil {
			return nil, err
		}
		return &ForgeResult{Action: action}, nil
	})
	t.Cleanup(func() { RegisterLiteForgeExecuteCallback(previous) })
}

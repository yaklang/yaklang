package aireact

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestReActSaveTimeline_BranchTimelineSkipped(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true))
	cfg.PersistentSessionId = "persistent-session-id"
	fork, err := cfg.Timeline.ForkForTask("1-1", "branch", cfg, cfg)
	require.NoError(t, err)
	require.NotNil(t, fork)
	cfg.Timeline = fork.Branch

	called := 0
	r := &ReAct{
		config: cfg,
		saveTimelineThrottle: func(fn func()) {
			called++
			// no-op on purpose: we only need to know whether save scheduling is attempted.
		},
	}
	r.SaveTimeline()
	require.Equal(t, 0, called)
}

func TestReActSaveTimeline_MainTimelineAllowed(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true))
	cfg.PersistentSessionId = "persistent-session-id"

	called := 0
	r := &ReAct{
		config: cfg,
		saveTimelineThrottle: func(fn func()) {
			called++
		},
	}
	r.SaveTimeline()
	require.Equal(t, 1, called)
}

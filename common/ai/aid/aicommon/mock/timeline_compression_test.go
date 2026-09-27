package mock

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// The old tests waited for Push to batch-compress and retain some raw items.
// The public contract now checks before a prompt and replaces all covered
// ordinary history in one transaction. Test the mock config's scheduler seam
// here; failure, concurrency, restore and precise state are covered by the
// aicommon timeline_compression_* tests.
func TestTimelineCompressionBeforePromptWithMockConfig(t *testing.T) {
	for _, limit := range []int64{100, 100000} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cfg := NewMockedAIConfig(ctx).(*MockedAIConfig)
			cfg.TimelineContentSizeLimit = limit
			tl := aicommon.NewTimeline(nil, nil)
			tl.SoftBindConfig(cfg, nil)
			calls := 0
			cfg.ScheduleAuxiliaryTaskFunc = func(ctx context.Context, name string, build func() string, onResult func(*aicommon.Action), opts ...aicommon.AuxiliaryTaskOption) {
				calls++
				require.Equal(t, aicommon.CallerLabelTimelineCompress, name)
				require.Contains(t, build(), "original history")
				spec := &aicommon.AuxiliaryTaskSpec{}
				for _, opt := range opts {
					opt(spec)
				}
				require.Equal(t, "timeline-summary", spec.OutputActionName)
				rsp, err := (&mockedAI{}).CallAI(aicommon.NewAIRequest(build()))
				require.NoError(t, err)
				action, err := aicommon.ExtractActionFromStream(ctx, rsp.GetOutputStreamReader(name, true, cfg.GetEmitter()), spec.OutputActionName)
				require.NoError(t, err)
				onResult(action)
			}
			for i := int64(1); i <= 20; i++ {
				tl.PushText(i, strings.Repeat("original history ", 20))
			}
			require.Len(t, tl.GetTimelineItemIDs(), 20)
			require.Zero(t, calls, "writes must never call AI or freeze")
			before := tl.Dump()
			require.Zero(t, calls, "rendering must also remain read-only")
			result, err := tl.CompressBeforePrompt(aicommon.TimelineCompressionOptions{Context: ctx})
			require.NoError(t, err)
			if limit == 100000 {
				require.Nil(t, result)
				require.Zero(t, calls)
				require.Equal(t, before, tl.Dump())
				return
			}
			require.NotNil(t, result)
			require.Equal(t, 1, calls)
			view := aicommon.RenderTimelineFrozenOpen(tl)
			require.Contains(t, view.Frozen, "Verified history")
			require.NotContains(t, view.Frozen, "original history")
			require.Empty(t, view.Open)
			require.EqualValues(t, 1, tl.FreezeSnapshot().Version)
			result, err = tl.CompressBeforePrompt(aicommon.TimelineCompressionOptions{Context: ctx})
			require.NoError(t, err)
			require.Nil(t, result)
			require.Equal(t, 1, calls)
			tl.PushText(21, "next observation")
			require.Contains(t, aicommon.RenderTimelineFrozenOpen(tl).Open, "next observation")
			require.Equal(t, view.Frozen, aicommon.RenderTimelineFrozenOpen(tl).Frozen)
		})
	}
}

package reactloops

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

func TestLoopToolCacheUsageHonorsProtocolOverride(t *testing.T) {
	tool := aitool.NewWithoutCallback("mode_probe", aitool.WithUsage(
		"[[- if .FunctionCallMode -]]NATIVE_ARGUMENTS[[- else -]]TEXT_ACTION[[- end -]]"))
	for _, parentNative := range []bool{false, true} {
		for _, loopNative := range []bool{false, true} {
			t.Run(fmt.Sprintf("parent=%t/loop=%t", parentNative, loopNative), func(t *testing.T) {
				cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
					aicommon.WithEnableFunctionCallMode(parentNative),
					aicommon.WithToolManager(buildinaitools.NewToolManager(buildinaitools.WithOnlyTools(tool))))
				loop := NewMinimalReActLoop(cfg, nil)
				WithFunctionCallMode(loopNative)(loop)
				require.NotNil(t, loop.RecordRecentlyUsedTool(tool).Upsert)
				want, absent := "TEXT_ACTION", "NATIVE_ARGUMENTS"
				if loopNative {
					want, absent = absent, want
				}
				view := aicommon.RenderTimelineFrozenOpen(cfg.Timeline).Open
				require.Contains(t, view, want)
				require.NotContains(t, view, absent)
				cfg.Timeline.FreezeAll()
				frozen := aicommon.RenderTimelineFrozenOpen(cfg.Timeline).PromotedRecentTools
				require.NotNil(t, loop.RecordRecentlyUsedTool(tool).Reuse)
				require.Equal(t, frozen, aicommon.RenderTimelineFrozenOpen(cfg.Timeline).PromotedRecentTools)
				require.Equal(t, parentNative, cfg.EnableFunctionCallMode)
			})
		}
	}
}

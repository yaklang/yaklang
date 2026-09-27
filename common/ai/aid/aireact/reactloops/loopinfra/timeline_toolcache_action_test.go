package loopinfra

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

func TestTimelineToolCacheActionResultBoundary(t *testing.T) {
	tool := aitool.NewWithoutCallback("result_probe", aitool.WithStringParam("value"))
	for _, tc := range []struct {
		name   string
		result *aitool.ToolResult
		err    error
		cached bool
	}{
		{"success", &aitool.ToolResult{Success: true}, nil, true},
		{"not settled", nil, nil, false},
		{"failed result", &aitool.ToolResult{Success: false}, nil, false},
		{"transport error despite result", &aitool.ToolResult{Success: true}, errors.New("interrupted"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
				aicommon.WithToolManager(buildinaitools.NewToolManager(buildinaitools.WithOnlyTools(tool))))
			recordSuccessfulToolCache(cfg, tool.Name, tc.result, tc.err)
			require.Equal(t, tc.cached, cfg.GetAiToolManager().IsRecentlyUsedTool(tool.Name))
			open := aicommon.RenderTimelineFrozenOpen(cfg.Timeline).Open
			if tc.cached {
				require.Contains(t, open, "[UPSERT] result_probe")
			} else {
				require.Empty(t, open)
			}
		})
	}
}

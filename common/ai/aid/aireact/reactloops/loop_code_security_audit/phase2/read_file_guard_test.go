package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestClampPhase2ReadFileParams_SearchPhaseFullRead(t *testing.T) {
	params := aitool.InvokeParams{
		"file":      "/tmp/a.go",
		"lines":     0,
		"mode":      "auto",
		"offset":    0,
		"line-size": 1024,
	}
	clamped, note := clampPhase2ReadFileParams(ScanPhaseSearch, params)
	require.Contains(t, note, "阶段A禁止全文件")
	require.Equal(t, 1, int(clamped.GetInt("offset")))
	require.Equal(t, phase2SearchReadFileMaxLines, int(clamped.GetInt("lines")))
	require.Equal(t, "lines", clamped.GetString("mode"))
	require.Empty(t, clamped.GetString("line-size"))
}

func TestClampPhase2ReadFileParams_SearchPhaseWithinLimit(t *testing.T) {
	params := aitool.InvokeParams{
		"file":   "/tmp/a.go",
		"offset": 10,
		"lines":  60,
		"mode":   "lines",
	}
	clamped, note := clampPhase2ReadFileParams(ScanPhaseSearch, params)
	require.Empty(t, note)
	require.Equal(t, params, clamped)
}

func TestClampPhase2ReadFileParams_SearchPhaseExceedsLimit(t *testing.T) {
	params := aitool.InvokeParams{
		"file":   "/tmp/a.go",
		"offset": 1,
		"lines":  300,
		"mode":   "lines",
	}
	clamped, note := clampPhase2ReadFileParams(ScanPhaseSearch, params)
	require.Contains(t, note, "限制为")
	require.Equal(t, phase2SearchReadFileMaxLines, int(clamped.GetInt("lines")))
}

func TestClampPhase2ReadFileParams_AuditPhaseAllowsFullRead(t *testing.T) {
	params := aitool.InvokeParams{
		"file":  "/tmp/a.go",
		"lines": 0,
		"mode":  "auto",
	}
	clamped, note := clampPhase2ReadFileParams(ScanPhaseAudit, params)
	require.Empty(t, note)
	require.Equal(t, params, clamped)
}

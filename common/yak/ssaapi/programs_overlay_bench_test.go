package ssaapi_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestOverlay_OverriddenFilesExcludedFromBaseRef asserts that after a file is
// overridden by the diff layer, overlay SF for that file uses the upper layer
// while unchanged files still resolve from base.
func TestOverlay_OverriddenFilesExcludedFromBaseRef(t *testing.T) {
	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Base";
}`,
				"Keep.java": `
public class Keep {
  static string keepStr = "Keep from Base";
}`,
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Diff";
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				require.NotNil(t, overlay)
				require.GreaterOrEqual(t, overlay.GetLayerCount(), 2)

				// A.java is overridden → not base-only; Keep.java remains base-only.
				require.False(t, overlay.IsBaseOnlyFile("/A.java"), "A.java should be overridden")
				require.True(t, overlay.IsBaseOnlyFile("/Keep.java") || overlay.IsBaseOnlyFile("Keep.java"),
					"Keep.java should remain base-only")

				res, err := overlay.SyntaxFlowWithError(`valueStr as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, res, map[string][]string{
					"res": {"Value from Diff"},
				})

				keep, err := overlay.SyntaxFlowWithError(`keepStr as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, keep, map[string][]string{
					"res": {"Keep from Base"},
				})
			},
		},
	)
}

func TestOverlay_RelocateUsesDirectLookup(t *testing.T) {
	var baseValue *ssaapi.Value
	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Base";
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage != ssatest.IncrementalCheckStageDB {
					return
				}
				vals := overlay.Ref("valueStr")
				require.NotEmpty(t, vals)
				baseValue = vals[0]
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Diff";
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage == ssatest.IncrementalCheckStageDB {
					return
				}
				require.NotNil(t, baseValue)
				start := time.Now()
				relocated := overlay.Relocate(baseValue)
				elapsed := time.Since(start)
				require.NotNil(t, relocated)
				require.NotEqual(t, baseValue.GetProgramName(), relocated.GetProgramName())
				// Direct lookup should be fast (nested SF used to be much slower).
				require.Less(t, elapsed, 5*time.Second, "Relocate should use direct SSA lookup")
			},
		},
	)
}

package ssaapi_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestOverlayDualSourceRefLiveIR verifies unchanged base files resolve from base
// IR and overridden files from the owner diff layer — without audit cache merge.
func TestOverlayDualSourceRefLiveIR(t *testing.T) {
	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{
			Files: map[string]string{
				"Keep.java": `
public class Keep {
  static string keepStr = "Keep from Base";
}`,
				"A.java": `
public class A {
  static string valueStr = "Value from Base";
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
				if stage != ssatest.IncrementalCheckStageCompile && stage != ssatest.IncrementalCheckStageDB {
					return
				}
				require.NotNil(t, overlay)

				ownedDiff := overlay.PathsOwnedByLayer(2)
				require.Contains(t, ownedDiff, "/A.java")
				require.NotContains(t, ownedDiff, "/Keep.java")
				require.Contains(t, overlay.PathsOwnedByLayer(1), "/Keep.java")

				keep, err := overlay.SyntaxFlowWithError(`keepStr as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, keep, map[string][]string{
					"res": {"Keep from Base"},
				})

				diff, err := overlay.SyntaxFlowWithError(`valueStr as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, diff, map[string][]string{
					"res": {"Value from Diff"},
				})
			},
		},
	)
}

package ssaapi_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestOverlay_RefMatchCreate_Regression covers Create (NewProgramOverLay / extend),
// Ref, and Match (Exact/Glob) against dual-source include/exclude ownership.
func TestOverlay_RefMatchCreate_Regression(t *testing.T) {
	ssatest.CheckIncrementalProgram(t,
		// step0: base create (single-program overlay via harness)
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
				"Gone.java": `
public class Gone {
  static string goneStr = "Gone from Base";
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage != ssatest.IncrementalCheckStageCompile && stage != ssatest.IncrementalCheckStageDB {
					return
				}
				require.NotNil(t, overlay)
				require.NotNil(t, overlay.Base)
				require.NotEmpty(t, overlay.Diff, "Create must produce at least one Diff layer")

				keepRefs := overlay.Ref("keepStr")
				require.NotEmpty(t, keepRefs)
				valueRefs := overlay.Ref("valueStr")
				require.NotEmpty(t, valueRefs)
				goneRefs := overlay.Ref("goneStr")
				require.NotEmpty(t, goneRefs)

				ok, vals, err := overlay.ExactMatch(context.Background(), ssadb.NameMatch, "keepStr")
				require.NoError(t, err)
				require.True(t, ok)
				require.False(t, vals.IsEmpty())

				ok, vals, err = overlay.GlobMatch(context.Background(), ssadb.NameMatch, "*Str")
				require.NoError(t, err)
				require.True(t, ok)
				require.False(t, vals.IsEmpty())
			},
		},
		// step1: override A, delete Gone — Create via extendOverlayWithNewLayer
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Diff1";
}`,
				"Gone.java": "",
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage != ssatest.IncrementalCheckStageCompile && stage != ssatest.IncrementalCheckStageDB {
					return
				}
				require.NotNil(t, overlay)
				require.GreaterOrEqual(t, len(overlay.Diff), 1)
				require.GreaterOrEqual(t, overlay.ProgramCount(), 2)

				require.Contains(t, overlay.Diff[len(overlay.Diff)-1].File, "/A.java")
				require.True(t, overlay.IsExcludedPath("/A.java"))
				require.True(t, overlay.IsExcludedPath("/Gone.java"), "deleted path must land in ExcludeFile")
				require.False(t, overlay.IsExcludedPath("/Keep.java"))
				require.Contains(t, overlay.ExcludeFile, "/A.java")
				require.Contains(t, overlay.ExcludeFile, "/Gone.java")

				// Ref: unchanged base file vs owned diff vs deleted
				keepRefs := overlay.Ref("keepStr")
				require.NotEmpty(t, keepRefs)
				valueRefs := overlay.Ref("valueStr")
				require.NotEmpty(t, valueRefs)
				require.Empty(t, overlay.Ref("goneStr"))

				keepSF, err := overlay.SyntaxFlowWithError(`keepStr as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, keepSF, map[string][]string{
					"res": {"Keep from Base"},
				})
				valueSF, err := overlay.SyntaxFlowWithError(`valueStr as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, valueSF, map[string][]string{
					"res": {"Value from Diff1"},
				})

				// Match: Exact/Glob see Diff override and skip deleted
				ok, vals, err := overlay.ExactMatch(context.Background(), ssadb.NameMatch, "valueStr")
				require.NoError(t, err)
				require.True(t, ok)
				require.False(t, vals.IsEmpty())

				ok, vals, err = overlay.ExactMatch(context.Background(), ssadb.NameMatch, "goneStr")
				require.NoError(t, err)
				require.False(t, ok)
				require.True(t, vals == nil || vals.IsEmpty())

				ok, vals, err = overlay.GlobMatch(context.Background(), ssadb.NameMatch, "keep*")
				require.NoError(t, err)
				require.True(t, ok)
				require.False(t, vals.IsEmpty())

				ok, vals, err = overlay.GlobMatch(context.Background(), ssadb.NameMatch, "gone*")
				require.NoError(t, err)
				require.False(t, ok)
				require.True(t, vals == nil || vals.IsEmpty())
			},
		},
		// step2: re-add Gone under a new layer; override A again
		ssatest.IncrementalStep{
			Files: map[string]string{
				"A.java": `
public class A {
  static string valueStr = "Value from Diff2";
}`,
				"Gone.java": `
public class Gone {
  static string goneStr = "Gone from Diff2";
}`,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				if stage != ssatest.IncrementalCheckStageCompile && stage != ssatest.IncrementalCheckStageDB {
					return
				}
				require.NotNil(t, overlay)
				require.GreaterOrEqual(t, len(overlay.Diff), 2, "extend Create must stack Diff layers")
				require.GreaterOrEqual(t, overlay.ProgramCount(), 3)

				top := overlay.Diff[len(overlay.Diff)-1]
				require.Contains(t, top.File, "/A.java")
				require.Contains(t, top.File, "/Gone.java")
				require.True(t, overlay.IsExcludedPath("/A.java"))
				require.True(t, overlay.IsExcludedPath("/Gone.java"))
				require.False(t, overlay.IsExcludedPath("/Keep.java"))

				require.NotEmpty(t, overlay.Ref("keepStr"))
				require.NotEmpty(t, overlay.Ref("valueStr"))
				require.NotEmpty(t, overlay.Ref("goneStr"))

				valueSF, err := overlay.SyntaxFlowWithError(`valueStr as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, valueSF, map[string][]string{
					"res": {"Value from Diff2"},
				})
				goneSF, err := overlay.SyntaxFlowWithError(`goneStr as $res`)
				require.NoError(t, err)
				ssatest.CompareResult(t, true, goneSF, map[string][]string{
					"res": {"Gone from Diff2"},
				})

				ok, vals, err := overlay.ExactMatch(context.Background(), ssadb.NameMatch, "goneStr")
				require.NoError(t, err)
				require.True(t, ok)
				require.False(t, vals.IsEmpty())

				ok, vals, err = overlay.GlobMatch(context.Background(), ssadb.NameMatch, "*Str")
				require.NoError(t, err)
				require.True(t, ok)
				require.False(t, vals.IsEmpty())
			},
		},
	)
}

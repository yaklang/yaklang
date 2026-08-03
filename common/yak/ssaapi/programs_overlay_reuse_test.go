package ssaapi_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestOverlaySkipBaseLayerWhenBaseCacheExists(t *testing.T) {
	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{
			Files: map[string]string{
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
				if stage != ssatest.IncrementalCheckStageDB {
					return
				}
				require.NotNil(t, overlay)

				baseProg := overlay.Layers[0].Program
				require.NotNil(t, baseProg)

				rule := `keepStr as $res
alert $res for {
	name: "keep_alert"
}`
				_, err := baseProg.SyntaxFlowWithError(rule, ssaapi.QueryWithSave(schema.SFResultKindScan))
				require.NoError(t, err)

				baseName := overlay.ResolveReuseBaseProgramName()
				require.NotEmpty(t, baseName)
				_, err = ssaapi.LoadResultByRuleContent(baseName, rule, schema.SFResultKindScan)
				require.NoError(t, err)

				res, err := overlay.SyntaxFlowWithError(rule, ssaapi.QueryWithSave(schema.SFResultKindScan))
				require.NoError(t, err)
				require.NotEmpty(t, res.GetGRPCModelRisk(), "base-only risks should merge from cached base scan")
			},
		},
	)
}

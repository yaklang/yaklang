package ssaapi_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestProgramSyntaxFlow_JoinsScanRuntime proves a direct program syntaxflow
// joins the same collect as a scan: the runtime receives the finding and the
// query writes no risk row itself.
func TestProgramSyntaxFlow_JoinsScanRuntime(t *testing.T) {
	progName := uuid.NewString()
	prog, err := ssaapi.Parse(`println(123)`,
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramName(progName),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	})

	rt := ssaapi.NewScanRuntime()
	var submitted []schema.RiskUpdateItem
	rt.ListenRisk(schema.RiskUpdateHandlerFunc(func(item schema.RiskUpdateItem) error {
		submitted = append(submitted, item)
		return nil
	}))

	res, err := prog.SyntaxFlowWithError(`
desc(title: "runtime program scan")
println(* as $target)
alert $target
`,
		ssaapi.QueryWithScanRuntime(rt),
		ssaapi.QueryWithMemory(),
	)
	require.NoError(t, err)
	require.Greater(t, res.RiskCount(), 0)
	require.NotEmpty(t, submitted, "the program query must submit its finding to the runtime")
	require.NotEmpty(t, res.GetResultUUID(), "an in-memory result still has a stable identity")
	for _, item := range submitted {
		require.Equal(t, res.GetResultUUID(), item.Risk.ResultUUID,
			"the finding points back at the result that produced it")
	}

	var riskRows int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).
		Where("program_name = ?", progName).Count(&riskRows).Error)
	require.Zero(t, riskRows, "the query does not write risk rows when a runtime is attached")
}

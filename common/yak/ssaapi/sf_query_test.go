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

func TestSFSave(t *testing.T) {
	progName := uuid.NewString()
	code := `
println(123)
	`
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramName(progName),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}()
	require.NoError(t, err)

	// query first
	rule := `println( * as $a)`
	res, err := prog.SyntaxFlowWithError(rule, ssaapi.QueryWithSave(schema.SFResultKindDebug))
	require.NoError(t, err)
	resId := res.GetResultID()
	// check

	auditResult, err := ssadb.GetResultByID(resId)
	require.NoError(t, err)
	require.Equal(t, progName, auditResult.ProgramName)
	require.Equal(t, rule, auditResult.RuleContent)
	require.Equal(t, schema.SFResultKindDebug, auditResult.Kind)
}

func TestSFNoSaveRiskKeepsOutputWithoutPersistence(t *testing.T) {
	progName := uuid.NewString()
	taskID := uuid.NewString()
	prog, err := ssaapi.Parse(`println(123)`,
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramName(progName),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	})

	config, err := ssaconfig.NewCLIScanConfig(ssaconfig.WithNoSaveRisk(true))
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError(`
desc(title: "no save risk")
println(* as $target)
alert $target
`,
		ssaapi.QueryWithSSAConfig(config),
		ssaapi.QueryWithSave(schema.SFResultKindScan),
		ssaapi.QueryWithTaskID(taskID),
	)
	require.NoError(t, err)
	require.Greater(t, res.RiskCount(), 0)
	require.Equal(t, ssaconfig.SFResultSaveMemory, res.GetResultSaveKind())

	var auditRows int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.AuditResult{}).
		Where("program_name = ?", progName).Count(&auditRows).Error)
	require.Zero(t, auditRows)
	var riskRows int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).
		Where("runtime_id = ?", taskID).Count(&riskRows).Error)
	require.Zero(t, riskRows)
}

func TestSFQueryWithCache(t *testing.T) {
	progName := uuid.NewString()
	code := `
println(123)
	`
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramName(progName),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}()
	require.NoError(t, err)

	// query first
	rule := `println( * as $a)`
	res, err := prog.SyntaxFlowWithError(rule, ssaapi.QueryWithSave(schema.SFResultKindDebug))
	require.NoError(t, err)
	res1Id := res.GetResultID()

	// query second, run query and get other id
	res2, err := prog.SyntaxFlowWithError(rule, ssaapi.QueryWithSave(schema.SFResultKindDebug))
	require.NoError(t, err)
	res2Id := res2.GetResultID()

	require.NotEqual(t, res1Id, res2Id)

	t.Run("test query with cache ", func(t *testing.T) {
		// query third, use cache get second id
		res3, err := prog.SyntaxFlowWithError(rule, ssaapi.QueryWithSave(schema.SFResultKindDebug), ssaapi.QueryWithUseCache())
		require.NoError(t, err)
		res3Id := res3.GetResultID()

		require.Equal(t, res1Id, res3Id)
	})

	t.Run("test query with cache and process", func(t *testing.T) {

		process := float64(0.0)
		res3, err := prog.SyntaxFlowWithError(rule,
			ssaapi.QueryWithSave(schema.SFResultKindDebug),
			ssaapi.QueryWithUseCache(),
			ssaapi.QueryWithProcessCallback(func(f float64, s string) {
				if process < f {
					process = f
				}
			}),
		)
		require.NoError(t, err)
		res3Id := res3.GetResultID()

		require.Equal(t, res1Id, res3Id)

		require.Equal(t, float64(1.0), process)
	})
}

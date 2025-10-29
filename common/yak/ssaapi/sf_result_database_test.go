package ssaapi_test

import (
	"testing"

	"github.com/yaklang/yaklang/common/log"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"golang.org/x/exp/slices"
)

func queryAndSave(t *testing.T) (func(), *ssaapi.SyntaxFlowResult) {
	code := `
		f = (a) =>{
			return a
		}
		target = f(1)
		`
	// parse code
	programName := uuid.NewString()
	prog, err := ssaapi.Parse(code, ssaapi.WithProgramName(programName), ssaapi.WithLanguage(ssaconfig.Yak))
	require.NoError(t, err)
	require.NotNil(t, prog)

	// query syntaxflow
	res, err := prog.SyntaxFlowWithError(`
	// normal variable and un-name variable 
	f(* as $target) // 1, 2 
	// no value variable 
	bbbbbb as $a  // nil 
	`)
	require.NoError(t, err)
	require.NotNil(t, res)

	// save result
	resultID, err := res.Save(schema.SFResultKindDebug)
	require.NoError(t, err)
	_ = resultID
	return func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	}, res
}

func TestQueryAndSave(t *testing.T) {
	deleteProgram, res := queryAndSave(t)
	_ = deleteProgram
	defer deleteProgram()

	// get variable in db
	resVariable, err := ssadb.GetResultVariableByID(ssadb.GetDB(), res.GetResultID())
	require.NoErrorf(t, err, "resultID: %v", res.GetResultID())
	spew.Dump(resVariable)
	spew.Dump(res.GetAllVariable())
	require.Equal(t, 2, len(resVariable))

	want := res.GetAllVariable()
	// require.Equal(t, res.GetAllVariable(), got)
	// got := make(map[string]int)
	for _, v := range resVariable {
		if v.Name == "_" {
			continue
		}
		want, have := want.Get(v.Name)
		require.True(t, have)
		require.Equal(t, int(v.ValueNum), want)
	}

	// get value in db
	resValueID, err := ssadb.GetResultValueByVariable(ssadb.GetDB(), res.GetResultID(), "target")
	require.NoError(t, err)
	wantValue := res.GetValues("target")
	wantValueID := lo.Map(wantValue, func(v *ssaapi.Value, _ int) int64 { return v.GetId() })
	require.Equal(t, len(wantValue), len(resValueID))
	slices.Sort(resValueID)
	slices.Sort(wantValueID)
	require.Equal(t, wantValueID, resValueID)
}
func TestGetResultFromDB(t *testing.T) {
	deleteProgram, wantRes := queryAndSave(t)
	defer deleteProgram()
	_ = wantRes

	// get result from db
	gotRes, err := ssaapi.LoadResultByID(wantRes.GetResultID())
	require.NoError(t, err)
	_ = gotRes

	// get "variable" from db
	gotVariable := gotRes.GetAllVariable()
	log.Infof("gotVariable: %v", gotVariable)
	wantVariable := wantRes.GetAllVariable()
	require.Equal(t, 2, gotVariable.Len())
	wantVariable.ForEach(func(key string, got any) {
		want, have := gotVariable.Get(key)
		require.True(t, have)
		require.Equal(t, got, want)
	})

	// get value from db
	wantValue := wantRes.GetValues("target")
	gotValue := gotRes.GetValues("target")
	wnatValueID := lo.Map(wantValue, func(v *ssaapi.Value, _ int) int64 { return v.GetId() })
	gotValueID := lo.Map(gotValue, func(v *ssaapi.Value, _ int) int64 { return v.GetId() })
	require.Equal(t, 1, len(gotValue))
	require.Equal(t, len(wantValue), len(gotValue))
	slices.Sort(wnatValueID)
	slices.Sort(gotValueID)
	require.Equal(t, wnatValueID, gotValueID)
}

func TestGetResultVariableByID(t *testing.T) {
	code := `
		f(1)
		f(2)
		f(3)
		`
	// parse code
	programName := uuid.NewString()
	prog, err := ssaapi.Parse(code, ssaapi.WithProgramName(programName), ssaapi.WithLanguage(ssaconfig.Yak))
	require.NoError(t, err)
	require.NotNil(t, prog)

	// query syntaxflow
	res, err := prog.SyntaxFlowWithError(`
	f(* as $target) // 1, 2 
	alert $target
	`)
	require.NoError(t, err)
	require.NotNil(t, res)
	res.Show()

	// save result
	resultID, err := res.Save(schema.SFResultKindDebug)
	require.NoError(t, err)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	got, err := ssadb.GetResultVariableByID(ssadb.GetDB(), resultID)
	require.NoError(t, err)
	require.Equal(t, 2, len(got))
	want := []*ssadb.ResultVariable{
		{
			Name:     "_",
			HasRisk:  false,
			ValueNum: 3,
		},
		{
			Name:     "target",
			HasRisk:  true,
			ValueNum: 3,
		},
	}
	spew.Dump(got)
	require.Equal(t, want, got)
}

func TestRuleAlertMsg(t *testing.T) {
	code := `
	print("a")
	print(f())
	`
	progName := uuid.NewString()
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramName(progName),
	)
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	require.NoError(t, err)

	syntaxFlowCode := ` 
	print(* as $para)
	$para ?{! opcode: const} as $target
	alert $target for {
		"msg": "target is not const",
		"level": "warning",
		"type": "security",
	}
	`
	check := func(result *ssaapi.SyntaxFlowResult) {
		require.Equal(t, result.GetVariableNum(), 2)
		variables := make([]string, 2)

		result.GetAllVariable().ForEach(func(key string, value any) {
			variables = append(variables, key)
		})
		require.Contains(t, variables, "target")

		require.Contains(t, result.GetAlertVariables(), "target")
		require.NotNil(t, result.GetValues("target"))

		info, ok := result.GetAlertInfo("target")
		log.Infof("info: %v", info)
		require.True(t, ok)
		require.Equal(t, "target is not const", info.Msg)
		require.Equal(t, "middle", string(info.Severity))
		require.Equal(t, "security", string(info.Purpose))
	}

	// rule  db/memory  * result db/memory = 4

	t.Run("rule memory, result memory", func(t *testing.T) {
		res, err := prog.SyntaxFlowWithError(syntaxFlowCode)
		require.NoError(t, err)
		check(res)
	})

	t.Run("rule memory, result db", func(t *testing.T) {
		res, err := prog.SyntaxFlowWithError(syntaxFlowCode)
		require.NoError(t, err)

		resultID, err := res.Save(schema.SFResultKindDebug)
		defer ssadb.DeleteResultByID(resultID)
		require.NoError(t, err)

		resFromDB, err := ssaapi.LoadResultByID(resultID)
		require.NoError(t, err)
		check(resFromDB)
	})

	t.Run("rule db, result memory", func(t *testing.T) {
		ruleName := uuid.NewString() + ".sf"
		_, err := sfdb.CreateRuleByContent(ruleName, syntaxFlowCode, false)
		defer sfdb.DeleteRuleByRuleName(ruleName)
		require.NoError(t, err)

		res, err := prog.SyntaxFlowRuleName(ruleName)
		require.NoError(t, err)
		check(res)
	})

	t.Run("rule db, result db", func(t *testing.T) {
		ruleName := uuid.NewString() + ".sf"
		_, err := sfdb.CreateRuleByContent(ruleName, syntaxFlowCode, false)
		defer sfdb.DeleteRuleByRuleName(ruleName)
		require.NoError(t, err)

		res, err := prog.SyntaxFlowRuleName(ruleName)
		require.NoError(t, err)

		resultID, err := res.Save(schema.SFResultKindDebug)
		defer ssadb.DeleteResultByID(resultID)
		require.NoError(t, err)

		resFromDB, err := ssaapi.LoadResultByID(resultID)
		require.NoError(t, err)
		check(resFromDB)
	})
}

func TestRuleRisk(t *testing.T) {
	code := `
	print(f())
	`
	progName := uuid.NewString()
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramName(progName),
	)
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	require.NoError(t, err)

	syntaxFlowCode := ` 
	desc (
		title: "check print variable",
	)
	print(* as $para)
	$para ?{!opcode: const} as $target
	$para ?{ opcode: const} as $target2
	alert $target for {
		"msg": "target is not const",
		"level": "warning",
		"type": "security",
		"risk": "sqli",
	}
	alert $target2 for {
		"msg": "target is const",
		"level": "low",
		"type": "security",
	}
	`
	ruleName := uuid.NewString() + ".sf"
	_, err = sfdb.CreateRuleByContent(ruleName, syntaxFlowCode, false)
	defer sfdb.DeleteRuleByRuleName(ruleName)
	require.NoError(t, err)

	res, err := prog.SyntaxFlowRuleName(ruleName)
	require.NoError(t, err)

	taskID := uuid.NewString()
	resultID, err := res.Save(schema.SFResultKindDebug, taskID)

	t.Cleanup(func() {
		defer ssadb.DeleteResultByID(resultID)
		defer yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{taskID},
		})
	})

	require.NoError(t, err)

	// check result
	resultDB, err := ssadb.GetResultByID(resultID)
	require.NoError(t, err)
	require.Equal(t, resultDB.RiskCount, uint64(1))

	// check risk
	_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		RuntimeID: []string{taskID},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, len(risks))
	require.Equal(t, resultDB.RiskCount, uint64(len(risks)))
	risk := risks[0]
	require.Contains(t, risk.Details, "target is not const")
	require.Equal(t, "sqli", risk.RiskType)
	require.Equal(t, schema.SyntaxFlowSeverity("middle"), risk.Severity)
	require.Equal(t, "check print variable", risk.Title)
}

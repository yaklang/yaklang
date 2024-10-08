package ssaapi_test

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
	prog, err := ssaapi.Parse(code, ssaapi.WithProgramName(programName), ssaapi.WithLanguage(consts.Yak))
	require.NoError(t, err)
	require.NotNil(t, prog)

	// query syntaxflow
	res, err := prog.SyntaxFlowWithError(`
	// normal variable and un-name variable 
	f(* as $target)
	// no value variable 
	bbbbbb as $a 
	`)
	require.NoError(t, err)
	require.NotNil(t, res)

	// save result
	resultID := uuid.NewString()
	err = res.Save(resultID, "", nil, prog)
	require.NoError(t, err)
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
	require.NoErrorf(t, err, "resultID: %s", res.GetResultID())
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

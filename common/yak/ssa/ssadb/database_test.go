package ssadb_test

import (
	"sort"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestSqliteID(t *testing.T) {
	db := consts.GetGormProjectDatabase().Debug()
	projectName := uuid.NewString()
	id, _ := ssadb.RequireIrCode(db, projectName)
	id2, _ := ssadb.RequireIrCode(db, projectName)
	defer ssadb.DeleteProgram(db, projectName)

	require.Equal(t, id+1, id2)
}

func TestBuild(t *testing.T) {
	db := consts.GetGormProjectDatabase().Debug()
	programName := uuid.NewString()
	code := `
		a = 1
		b = a
		c = b
		d = c + a
		`

	prog, err := ssaapi.Parse(
		code,
		ssaapi.WithLanguage(ssaapi.Yak),
		ssaapi.WithDataBase(programName),
	)
	defer ssadb.DeleteProgram(db, programName)

	require.NoError(t, err)
	prog.Program.ShowWithSource()

	irCodes := ssadb.GetIrByVariable(db, programName, "a")
	require.Len(t, irCodes, 1, "a instruction count should be 1")

	irCode := irCodes[0]
	require.NotNil(t, irCode)

	spew.Dump(irCode)
	require.Equal(t, ssa.SSAOpcode2Name[ssa.SSAOpcodeConstInst], irCode.OpcodeName)
	require.Equal(t, "1", irCode.ConstantValue)

	v := irCode.Variable
	sort.Strings(v)
	require.Equal(t, ssadb.StringSlice{"a", "b", "c"}, v)
}

func TestBuild_Multiple_Program(t *testing.T) {
	db := consts.GetGormProjectDatabase().Debug()

	check := func(code, want string) {
		programName := uuid.NewString()

		prog, err := ssaapi.Parse(
			code,
			ssaapi.WithLanguage(ssaapi.Yak),
			ssaapi.WithDataBase(programName),
		)
		defer ssadb.DeleteProgram(db, programName)

		require.NoError(t, err)
		prog.Program.ShowWithSource()

		irCodes := ssadb.GetIrByVariable(db, programName, "a")
		require.Len(t, irCodes, 1, "a instruction count should be 1")

		irCode := irCodes[0]
		require.NotNil(t, irCode)

		require.NotNil(t, irCode)

		spew.Dump(irCode)
		require.Equal(t, ssa.SSAOpcode2Name[ssa.SSAOpcodeConstInst], irCode.OpcodeName)
		require.Equal(t, want, irCode.ConstantValue)
	}

	check(`a = 1`, "1")
	check(`a = 2`, "2")
}

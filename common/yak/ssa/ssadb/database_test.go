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

	ircode := ssadb.GetIrByVariable(db, programName, "a")

	require.NotNil(t, ircode)

	spew.Dump(ircode)
	require.Equal(t, ssa.SSAOpcode2Name[ssa.SSAOpcodeConstInst], ircode.OpcodeName)
	require.Equal(t, "1", ircode.ConstantValue)

	v := ircode.Variable
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

		ircode := ssadb.GetIrByVariable(db, programName, "a")

		require.NotNil(t, ircode)

		spew.Dump(ircode)
		require.Equal(t, ssa.SSAOpcode2Name[ssa.SSAOpcodeConstInst], ircode.OpcodeName)
		require.Equal(t, want, ircode.ConstantValue)
	}

	check(`a = 1`, "1")
	check(`a = 2`, "2")
}

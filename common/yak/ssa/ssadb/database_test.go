package ssadb_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestSqliteID(t *testing.T) {
	db := ssadb.GetDB().Debug()
	projectName := uuid.NewString()
	id, _ := ssadb.RequireIrCode(db, projectName)
	id2, _ := ssadb.RequireIrCode(db, projectName)
	defer ssadb.DeleteProgram(db, projectName)

	require.Greater(t, id2, id)
}

func TestBuild(t *testing.T) {
	db := ssadb.GetDB().Debug()
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
		ssaapi.WithProgramName(programName),
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

	v := irCode.Variable
	sort.Strings(v)
	require.Equal(t, ssadb.StringSlice{"a", "b", "c"}, v)
}

func TestBuild_Multiple_Program(t *testing.T) {
	db := ssadb.GetDB().Debug()

	check := func(code, variable string) {
		programName := uuid.NewString()

		prog, err := ssaapi.Parse(
			code,
			ssaapi.WithLanguage(ssaapi.Yak),
			ssaapi.WithProgramName(programName),
		)
		defer ssadb.DeleteProgram(db, programName)

		require.NoError(t, err)
		prog.Program.ShowWithSource()

		irCodes := ssadb.GetIrByVariable(db, programName, variable)
		require.Len(t, irCodes, 1, "a instruction count should be 1")

		irCode := irCodes[0]
		require.NotNil(t, irCode)

		require.NotNil(t, irCode)

		spew.Dump(irCode)
		require.Equal(t, ssa.SSAOpcode2Name[ssa.SSAOpcodeConstInst], irCode.OpcodeName)
		require.Equal(t, ssadb.StringSlice{variable}, irCode.Variable)
	}

	check(`a = 1`, "a")
	check(`b = 2`, "b")
}

func TestSyncFromDatabase(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		programName := uuid.NewString()
		// db := ssadb.GetDB()
		prog, err := ssaapi.Parse(`
		a = 1 
		print(a)
		`,
			ssaapi.WithLanguage(ssaapi.Yak),
			ssaapi.WithProgramName(programName),
		)
		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
		require.NoError(t, err)

		prog.Program.ShowWithSource()

		cache := prog.Program.Cache
		_ = cache
		valuesA := prog.Ref("a")
		require.Len(t, valuesA, 1)
		valueA := valuesA[0]
		// valueA.GetId()

		cache.SaveToDatabase()
		lazyInst := cache.GetInstruction(valueA.GetId())
		require.NotNil(t, lazyInst)

		lz, isLazyInstruction := ssa.ToLazyInstruction(lazyInst)
		// spew.Dump(lazyInst)
		require.True(t, isLazyInstruction)
		require.Equal(t, ssa.SSAOpcodeConstInst, lz.GetOpcode())

		fmt.Println("lz: ", lz.String())

		users := lz.GetUsers()
		fmt.Println("users: ", users)
		require.Len(t, users, 1)
		user := users[0]
		require.NotNil(t, user)
		require.Equal(t, ssa.SSAOpcodeCall, user.GetOpcode())
	})

}

func TestProgramRelation(t *testing.T) {
	// now no other program in database
	t.Skip()
	/*

		in program:
			a -> b -> c

		in up-down stream:
			a  -> b, c
			b  -> c
	*/
	ssadb.DeleteProgram(ssadb.GetDB(), "a")
	ssadb.DeleteProgram(ssadb.GetDB(), "b")
	ssadb.DeleteProgram(ssadb.GetDB(), "c")

	addStream := func(down, up *ssadb.IrProgram) {
		up.DownStream = append(up.DownStream, down.ProgramName)
		down.UpStream = append(down.UpStream, up.ProgramName)
	}
	a := ssadb.CreateProgram("a", "Application", "")
	b := ssadb.CreateProgram("b", "Library", "")
	c := ssadb.CreateProgram("c", "Library", "")
	/*
		a -> b, c
		b -> c
	*/
	addStream(a, b)
	addStream(a, c)
	addStream(b, c)
	ssadb.UpdateProgram(a)
	ssadb.UpdateProgram(b)
	ssadb.UpdateProgram(c)

	ssadb.DeleteProgram(ssadb.GetDB(), "a")

	// check all program should deleted
	{
		irProg, err := ssadb.GetProgram("a", "")
		assert.NotNilf(t, err, "a should be deleted")
		assert.Nilf(t, irProg, "a should be deleted")
	}
	{
		irProg, err := ssadb.GetProgram("b", "")
		assert.NotNilf(t, err, "b should be deleted")
		assert.Nilf(t, irProg, "b should be deleted")
	}
	{
		irProg, err := ssadb.GetProgram("c", "")
		assert.NotNilf(t, err, "b should be deleted")
		assert.Nilf(t, irProg, "b should be deleted")
	}

}

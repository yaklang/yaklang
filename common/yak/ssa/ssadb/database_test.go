package ssadb_test

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestSqliteID(t *testing.T) {
	db := ssadb.GetDB().Debug()
	projectName := uuid.NewString()
	// id, _ := ssadb.RequireIrCode(db, projectName)
	// id2, _ := ssadb.RequireIrCode(db, projectName)
	defer ssadb.DeleteProgram(db, projectName)

	// require.Greater(t, id2, id)
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
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithProgramName(programName),
	)
	defer ssadb.DeleteProgram(db, programName)

	require.NoError(t, err)
	prog.Program.ShowWithSource()

	// irCodes := ssadb.GetIrByVariable(db, programName, "a")
	// require.Len(t, irCodes, 1, "a instruction count should be 1")

	// irCode := irCodes[0]
	// require.NotNil(t, irCode)

	// spew.Dump(irCode)
	// require.Equal(t, ssa.SSAOpcode2Name[ssa.SSAOpcodeConstInst], irCode.OpcodeName)

	// v := irCode.Variable
	// sort.Strings(v)
	// require.Equal(t, ssadb.StringSlice{"a", "b", "c"}, v)
}

func TestBuild_Multiple_Program(t *testing.T) {
	db := ssadb.GetDB().Debug()

	check := func(code, variable string) {
		programName := uuid.NewString()

		prog, err := ssaapi.Parse(
			code,
			ssaapi.WithLanguage(ssaconfig.Yak),
			ssaapi.WithProgramName(programName),
		)
		defer ssadb.DeleteProgram(db, programName)

		require.NoError(t, err)
		prog.Program.ShowWithSource()

		// irCodes := ssadb.GetIrByVariable(db, programName, variable)
		// require.Len(t, irCodes, 1, "a instruction count should be 1")

		// irCode := irCodes[0]
		// require.NotNil(t, irCode)

		// require.NotNil(t, irCode)

		// spew.Dump(irCode)
		// require.Equal(t, ssa.SSAOpcode2Name[ssa.SSAOpcodeConstInst], irCode.OpcodeName)
		// require.Equal(t, ssadb.StringSlice{variable}, irCode.Variable)
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
			ssaapi.WithLanguage(ssaconfig.Yak),
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

		// cache.SaveToDatabase() // not allow save again
		inst := cache.GetInstruction(valueA.GetId())
		require.NotNil(t, inst)

		// lz, isLazyInstruction := ssa.ToLazyInstruction(lazyInst)
		// spew.Dump(lazyInst)
		// require.True(t, isLazyInstruction)
		require.Equal(t, ssa.SSAOpcodeConstInst, inst.GetOpcode())

		fmt.Println("lz: ", inst.String())

		if value, ok := ssa.ToValue(inst); ok {
			users := value.GetUsers()
			fmt.Println("users: ", users)
			require.Len(t, users, 1)
			user := users[0]
			require.NotNil(t, user)
			require.Equal(t, ssa.SSAOpcodeCall, user.GetOpcode())
		}

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

func TestLoadEditor(t *testing.T) {
	code := `package main; func main() { a := 1; print(a) }`
	filePath := "a.go"
	vf := filesys.NewVirtualFs()
	vf.AddFile(filePath, code)

	programName := uuid.NewString()
	// get prog
	_, err := ssaapi.ParseProject(
		ssaapi.WithProgramName(programName),
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithFileSystem(vf),
	)
	require.NoError(t, err)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	}()
	prog, err := ssaapi.FromDatabase(programName)
	require.NoError(t, err)

	// create template value
	// editor := memedit.NewMemEditor(code)
	// editor.SetUrl(filePath)
	print := prog.Ref("print")
	require.Len(t, print, 1)
	codeRange := print[0].GetRange()
	require.NotNil(t, codeRange)
	editor := codeRange.GetEditor()
	require.NotNil(t, editor)

	editorFromDB, err := ssadb.GetEditorByHash(editor.GetIrSourceHash())
	require.NoError(t, err)
	require.NotNil(t, editorFromDB)
	require.Equal(t, editor.GetUrl(), editorFromDB.GetUrl())
	require.Equal(t, editor.GetIrSourceHash(), editorFromDB.GetIrSourceHash())
}

func TestAuditResult(t *testing.T) {
	code := `package main; func main() { a := 1; print(a) }`
	filePath := "a.go"
	vf := filesys.NewVirtualFs()
	vf.AddFile(filePath, code)

	programName := uuid.NewString()
	taskId := uuid.NewString()
	// get prog
	_, err := ssaapi.ParseProject(
		ssaapi.WithProgramName(programName),
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithFileSystem(vf),
	)
	require.NoError(t, err)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	}()
	prog, err := ssaapi.FromDatabase(programName)
	require.NoError(t, err)

	// create template value
	// editor := memedit.NewMemEditor(code)
	// editor.SetUrl(filePath)
	print := prog.Ref("print")
	require.Len(t, print, 1)
	codeRange := print[0].GetRange()
	require.NotNil(t, codeRange)

	value := prog.NewConstValue("print", codeRange)
	require.NoError(t, err)

	// save memResult
	memResult := sfvm.NewSFResult(&schema.SyntaxFlowRule{}, &sfvm.Config{})
	memResult.SymbolTable.Set("print", value)
	result := ssaapi.CreateResultWithProg(prog, memResult)
	result.Show()
	resultId, err := result.Save(schema.SFResultKindSearch, taskId)
	require.NoError(t, err)

	log.Infof("resultId: %d", resultId)
	// load result and check template value
	dbResult, err := ssaapi.LoadResultByID(resultId)
	require.NoError(t, err)
	dbResult.Show()
	values := dbResult.GetValues("print")
	values.Show()
	require.True(t, len(values) != 0)
	values.Recursive(func(operator sfvm.ValueOperator) error {
		switch ret := operator.(type) {
		case *ssaapi.Value:
			require.True(t, ret.GetId() == -1)
			require.True(t, ret.GetRange() != nil)
			require.True(t, ret.GetRange().String() == codeRange.String())
		}
		return nil
	})
}

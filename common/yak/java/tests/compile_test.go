package tests

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestCompileProgram_OnlySource(t *testing.T) {
	pkgName := "a" + strings.ReplaceAll(uuid.NewString(), "-", "")
	code := fmt.Sprintf(`
package %s; 
public class A {
	int a;
} 
	`, pkgName)
	ssadb.DeleteProgram(ssadb.GetDB(), pkgName)

	// compile only source
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JAVA))
	assert.NoError(t, err)
	prog.Show()
	assert.Equal(t, 1, len(prog.Program.UpStream))
	assert.Equal(t, 0, len(prog.Program.DownStream))
	if slices.Contains(ssadb.AllPrograms(ssadb.GetDB()), pkgName) {
		t.Fatalf("package %s should not be in the database", pkgName)
	}
}
func TestCompileProgram_WithDatabase(t *testing.T) {
	pkgName := "a" + strings.ReplaceAll(uuid.NewString(), "-", "")
	code := fmt.Sprintf(`
package %s; 
public class A {
	int a;
} 
	`, pkgName)
	ssadb.DeleteProgram(ssadb.GetDB(), pkgName)
	// compile with database
	programId := uuid.NewString()
	ssadb.DeleteProgram(ssadb.GetDB(), programId)

	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage(ssaapi.JAVA),
		ssaapi.WithDatabaseProgramName(programId),
	)
	assert.NoError(t, err)

	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programId)
		ssadb.DeleteProgram(ssadb.GetDB(), pkgName)
	}()

	prog.Show()
	assert.Equal(t, 1, len(prog.Program.UpStream))
	assert.Equal(t, 0, len(prog.Program.DownStream))
	{
		// check in ir-code table
		var programsInIrCode []string
		ssadb.GetDB().Model(&ssadb.IrCode{}).Select("DISTINCT(program_name)").Pluck("program_name", &programsInIrCode)
		if !slices.Contains(programsInIrCode, pkgName) {
			t.Fatalf("package %s should be in the ir-code table", pkgName)
		}

		// check in ir-program table
		var programsInIrProgram []string
		ssadb.GetDB().Model(&ssadb.IrProgram{}).Select("DISTINCT(program_name)").Pluck("program_name", &programsInIrProgram)
		if !slices.Contains(programsInIrProgram, pkgName) {
			t.Fatalf("package %s should be in the ir-program table", pkgName)
		}
	}

	// compare program-application with database
	{
		var out ssadb.IrProgram
		err = ssadb.GetDB().Model(&ssadb.IrProgram{}).Where(
			"program_name = ?", programId,
		).First(&out).Error
		assert.NoError(t, err)
		log.Infof("out programID: %v", out)
		assert.Equal(t, programId, out.ProgramName)
		assert.Equal(t, 1, len(out.UpStream))
		assert.Equal(t, 0, len(out.DownStream))
		assert.Equal(t, string(ssa.Application), out.ProgramKind)
	}
	// compare package-library with database
	{
		var out ssadb.IrProgram
		err = ssadb.GetDB().Model(&ssadb.IrProgram{}).Where(
			"program_name = ?", pkgName,
		).First(&out).Error
		assert.NoError(t, err)
		log.Infof("out pkgName: %v", out)
		assert.Equal(t, pkgName, out.ProgramName)
		assert.Equal(t, 0, len(out.UpStream))
		assert.Equal(t, 1, len(out.DownStream))
		assert.Equal(t, string(ssa.Library), out.ProgramKind)
	}
}

func TestCompileProgram_OnlyDatabase(t *testing.T) {
	pkgName := "a" + strings.ReplaceAll(uuid.NewString(), "-", "")
	programId := uuid.NewString()
	{
		code := fmt.Sprintf(`
	package %s; 
	public class A {
		int a;
	} 
		`, pkgName)
		ssadb.DeleteProgram(ssadb.GetDB(), pkgName)
		// compile with database
		ssadb.DeleteProgram(ssadb.GetDB(), programId)

		_, err := ssaapi.Parse(code,
			ssaapi.WithLanguage(ssaapi.JAVA),
			ssaapi.WithDatabaseProgramName(programId),
		)
		assert.NoError(t, err)

		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programId)
			ssadb.DeleteProgram(ssadb.GetDB(), pkgName)
		}()
	}

	prog, err := ssaapi.FromDatabase(programId)
	assert.NoError(t, err)
	prog.Show()
	// assert.Equal(t, 1, len(prog.Program.UpStream))
	// assert.Equal(t, 0, len(prog.Program.DownStream))
	// for name := range prog.Program.UpStream {
	// 	if name != pkgName {
	// 		t.Fatalf("upstream should be %s, but got %s", pkgName, name)
	// 	}
	// }
}

func TestCompileProgram_Delete(t *testing.T) {

	pkgName := "a" + strings.ReplaceAll(uuid.NewString(), "-", "")
	programId := uuid.NewString()
	// compile
	{
		code := fmt.Sprintf(`
	package %s; 
	public class A {
		int a;
	} 
		`, pkgName)
		ssadb.DeleteProgram(ssadb.GetDB(), pkgName)
		// compile with database
		ssadb.DeleteProgram(ssadb.GetDB(), programId)

		_, err := ssaapi.Parse(code,
			ssaapi.WithLanguage(ssaapi.JAVA),
			ssaapi.WithDatabaseProgramName(programId),
		)
		assert.NoError(t, err)
	}

	// delete
	ssadb.DeleteProgram(ssadb.GetDB(), programId)

	// check in ir-code table
	var programsInIrCode []string
	ssadb.GetDB().Model(&ssadb.IrCode{}).Select("DISTINCT(program_name)").Pluck("program_name", &programsInIrCode)
	if slices.Contains(programsInIrCode, pkgName) {
		t.Fatalf("package %s should not be in the ir-code table", pkgName)
	}

	// check in ir-program table
	var programsInIrProgram []string
	ssadb.GetDB().Model(&ssadb.IrProgram{}).Select("DISTINCT(program_name)").Pluck("program_name", &programsInIrProgram)
	if slices.Contains(programsInIrProgram, pkgName) {
		t.Fatalf("package %s should not be in the ir-program table", pkgName)
	}
}

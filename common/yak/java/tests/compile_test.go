package tests

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"golang.org/x/exp/slices"
)

func comparePackage(prog *ssadb.IrProgram) error {
	// compare package-library with database
	var out ssadb.IrProgram
	err := ssadb.GetDB().Model(&ssadb.IrProgram{}).Where(
		"program_name = ?", prog.ProgramName,
	).First(&out).Error
	if err != nil {
		return utils.Errorf("get program %s error : %v", prog.ProgramName, err)
	}
	log.Infof("out pkgName: %v", out)
	if out.ProgramName != prog.ProgramName {
		return fmt.Errorf("program name not match want %v, got %v", prog.ProgramName, out.ProgramName)
	}
	if prog.ProgramKind != out.ProgramKind {
		return fmt.Errorf("program kind not match want %v, got %v", prog.ProgramKind, out.ProgramKind)
	}
	if slices.Compare(prog.UpStream, out.UpStream) != 0 {
		return fmt.Errorf("upstream not match want %v, got %v", prog.UpStream, out.UpStream)
	}
	if slices.Compare(prog.DownStream, out.DownStream) != 0 {
		return fmt.Errorf("downstream not match want %v, got %v", prog.DownStream, out.DownStream)
	}
	return nil
}

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
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.JAVA))
	assert.NoError(t, err)
	prog.Show()
	assert.Equal(t, 1, prog.Program.UpStream.Len())
	assert.Equal(t, 0, len(prog.Program.DownStream))
	if slices.Contains(ssadb.AllProgramNames(ssadb.GetDB()), pkgName) {
		t.Fatalf("package %s should not be in the database", pkgName)
	}
}
func TestCompileProgram_WithDatabase(t *testing.T) {
	// this test want library save in db, but now only save application
	t.Skip()
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
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(programId),
	)
	assert.NoError(t, err)

	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programId)
		ssadb.DeleteProgram(ssadb.GetDB(), pkgName)
	}()

	prog.Show()
	assert.Equal(t, 1, prog.Program.UpStream.Len())
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
	if err := comparePackage(&ssadb.IrProgram{
		ProgramName: programId,
		ProgramKind: (ssa.Application),
		UpStream:    []string{pkgName},
		DownStream:  []string{},
	}); err != nil {
		t.Fatalf("compare package failed: %v", err)
	}
	// compare package-library with database
	if err := comparePackage(&ssadb.IrProgram{
		ProgramName: pkgName,
		ProgramKind: (ssa.Library),
		UpStream:    []string{},
		DownStream:  []string{programId},
	}); err != nil {
		t.Fatalf("compare package failed: %v", err)
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
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programId),
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
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programId),
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
func TestCompileProgram_ReUseLibrary(t *testing.T) {
	// TODO: re-write this test case
	t.Skip()

	pkgName := "a" + strings.ReplaceAll(uuid.NewString(), "-", "")
	code := fmt.Sprintf(`
	package %s; 
	public class A {
		public static void main(String[] args) {
			int a = 1;
		}
	} 
		`, pkgName)

	// compile with database
	// compile
	programID1 := uuid.NewString()
	{
		ssadb.DeleteProgram(ssadb.GetDB(), programID1)
		_, err := ssaapi.Parse(code,
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programID1),
		)
		defer ssadb.DeleteProgram(ssadb.GetDB(), programID1)
		assert.NoError(t, err)
	}
	if err := comparePackage(&ssadb.IrProgram{
		ProgramName: pkgName,
		ProgramKind: (ssa.Library),
		UpStream:    []string{},
		DownStream:  []string{programID1},
	}); err != nil {
		t.Fatalf("compare package failed: %v", err)
	}
	programID2 := uuid.NewString()
	{
		ssadb.DeleteProgram(ssadb.GetDB(), programID2)
		_, err := ssaapi.Parse(code,
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programID2),
		)
		defer ssadb.DeleteProgram(ssadb.GetDB(), programID2)
		assert.NoError(t, err)
	}

	// check ir-package re-use
	if err := comparePackage(&ssadb.IrProgram{
		ProgramName: pkgName,
		ProgramKind: (ssa.Library),
		UpStream:    []string{},
		DownStream:  []string{programID1, programID2},
	}); err != nil {
		t.Fatalf("compare package failed: %v", err)
	}

	// query
	prog, err := ssaapi.FromDatabase(programID2)
	assert.NoError(t, err)

	res := prog.SyntaxFlow(`a as $a`, ssaapi.QueryWithEnableDebug())
	assert.Equal(t,
		[]string{"1"},
		lo.Map(
			res.GetValues("a"),
			func(v *ssaapi.Value, _ int) string { return v.String() },
		),
	)

	// delete
	// delete program 1, will not affect program 2
	ssadb.DeleteProgram(ssadb.GetDB(), programID1)
	if err := comparePackage(&ssadb.IrProgram{
		ProgramName: pkgName,
		ProgramKind: (ssa.Library),
		UpStream:    []string{},
		DownStream:  []string{programID2},
	}); err != nil {
		t.Fatalf("compare package failed: %v", err)
	}

	// delete program 2, pkg should be deleted
	ssadb.DeleteProgram(ssadb.GetDB(), programID2)
	var programsInIrProgram []string
	ssadb.GetDB().Model(&ssadb.IrProgram{}).Select("DISTINCT(program_name)").Pluck("program_name", &programsInIrProgram)
	if slices.Contains(programsInIrProgram, pkgName) {
		t.Fatalf("package %s should not be in the ir-program table", pkgName)
	}

}

func TestCompileProgram_MultipleFileInLibrary(t *testing.T) {
	// TODO: re-write this test case
	t.Skip()
	pkgName := "a" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ssadb.DeleteProgram(ssadb.GetDB(), pkgName)
	vf := filesys.NewVirtualFs()
	vf.AddFile("org/pom.xml", `
	<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    <dependencies>
        <dependency>
            <groupId>xerial</groupId>
            <artifactId>sqlite-jdbc</artifactId>
            <version>3.36.0.3</version>  <!-- 请检查最新版本 -->
        </dependency>
	</dependen
	`)

	vf.AddFile("org/main/Main.java", fmt.Sprintf(`
	package %s;
	public class Main {
		public static void main(String[] args) {
			int a = 1;
		}
	}
	`, pkgName))
	vf.AddFile("org/main/Utils.java", fmt.Sprintf(`
	package %s;
	public class Utils {
		public static void A() {
			int a = 2;
		}
	}
	`, pkgName))

	downStream := []string{}
	defer func() {
		for _, programID := range downStream {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
		}
	}()
	pkgFileLen := 2

	check := func(programID string, want []string) {
		ssadb.DeleteProgram(ssadb.GetDB(), programID)
		prog, err := ssaapi.ParseProjectWithFS(
			vf,
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programID),
		)
		assert.NoError(t, err)
		// prog.Show()
		_ = prog

		downStream = append(downStream, programID)
		irProg, err := ssadb.GetProgram(pkgName, ssa.Library)
		assert.NoError(t, err)
		assert.Equal(t, downStream, []string(irProg.DownStream))
		assert.Equal(t, len(irProg.FileList), pkgFileLen)

		{
			prog, err := ssaapi.FromDatabase(programID)
			assert.NoError(t, err)
			_ = prog
			res, err := prog.SyntaxFlowWithError(`a as $a`, ssaapi.QueryWithEnableDebug())
			assert.NoError(t, err)
			assert.Equal(t,
				want,
				lo.Map(
					res.GetValues("a"),
					func(v *ssaapi.Value, _ int) string { return v.String() },
				),
			)
		}
	}

	// compile with database
	t.Run("compile and test", func(t *testing.T) {
		check(uuid.NewString(), []string{"1", "2"})
	})
	// re-build, test package re-use
	t.Run("re-compile and test re-use", func(t *testing.T) {
		check(uuid.NewString(), []string{"1", "2"})
		check(uuid.NewString(), []string{"1", "2"})
	})

	t.Run("re-compile add file", func(t *testing.T) {
		check(uuid.NewString(), []string{"1", "2"})
		vf.AddFile("org/main/B.java", fmt.Sprintf(`
		package %s;
		public class B {
			public static void A() {
				int a = 3;
			}
		}
		`, pkgName))
		pkgFileLen++
		check(uuid.NewString(), []string{"1", "2", "3"})
	})

}

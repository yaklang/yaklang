package ssaapi_test

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/log"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func Test_CompileError(t *testing.T) {
	check := func(t *testing.T, fs filesys_interface.FileSystem) {
		progName := uuid.NewString()
		finalProcess := 0.0
		prog, err := ssaapi.ParseProjectWithFS(fs,
			ssaapi.WithStrictMode(true),
			ssaapi.WithProgramName(progName),
			ssaapi.WithProcess(func(msg string, process float64) {
				if process > finalProcess {
					finalProcess = process
				}
			}))
		defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
		log.Error(err)
		require.Error(t, err)
		_ = prog
		require.Less(t, finalProcess, 1.0)

		irProg, err := ssadb.GetProgram(progName, ssa.Application)
		require.Nil(t, irProg)
		require.Error(t, err)
		require.True(t, gorm.IsRecordNotFoundError(err))

	}

	t.Run("test compile error single file  ", func(t *testing.T) {
		// Compile the file
		vf := filesys.NewVirtualFs()
		vf.AddFile("b.yak", "print('Hello, ")
		check(t, vf)
	})

	t.Run("test compile error with fast fail ", func(t *testing.T) {
		// Compile the file
		vf := filesys.NewVirtualFs()
		vf.AddFile("a.yak", "print('Hello,')")
		vf.AddFile("b.yak", "print('Hello, ")
		vf.AddFile("c.yak", "print('Hello,')")
		check(t, vf)
	})

	t.Run("test compile error in single file in dir", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("a.yak", "print('Hello,')")
		vf.AddFile("bb/b.yak", "print('Hello, ")
		check(t, vf)
	})

	t.Run("test compile error without fast fail  ", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("a.yak", "print('Hello,')")
		vf.AddFile("b.yak", "print('Hello, ")
		vf.AddFile("c.yak", "print('Hello,')")
		finalProcess := 0.0
		prog, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithProcess(func(msg string, process float64) {
			if process > finalProcess {
				finalProcess = process
			}
		}))
		require.NoError(t, err)
		_ = prog
		require.Equal(t, finalProcess, 1.0)
	})

	t.Run("test compile normal   ", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("a.yak", "print('Hello,')")
		vf.AddFile("b.yak", "print('Hello, )")
		vf.AddFile("c.yak", "print('Hello,')")
		finalProcess := 0.0
		prog, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithProcess(func(msg string, process float64) {
			if process > finalProcess {
				finalProcess = process
			}
		}))
		require.NoError(t, err)
		_ = prog
		require.Equal(t, finalProcess, 1.0)
	})

}

func TestJava_ProcessManage_Mutli_Files(t *testing.T) {
	toCreate := []string{
		"example/src/main/java/com/example/testcontextB/a.java",
		"example/src/main/java/com/example/testcontextB/b.java",
		"example/src/main/java/com/example/testcontextA/a.java",
		"example/src/main/java/com/example/testcontextA/b.java",
		"example/src/main/java/com/example/testcontextA/c.java",
		"example/src/main/java/com/example/testcontextA/d.java",
		"example/src/main/java/com/example/testcontextA/e.java",
		"example/src/main/java/com/example/testcontextC/a.java",
		"example/src/main/java/com/example/testcontextC/b.java",
		"example/src/main/java/com/example/testcontextC/c.java",
	}

	t.Run("test mutli files stop process by context", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		for _, path := range toCreate {
			dirPath := filepath.Dir(path)
			lastDirName := filepath.Base(dirPath)
			vf.AddFile(path, fmt.Sprintf("package %s;", lastDirName))
		}

		var maxProcess float64
		programID := uuid.NewString()
		ctx, cancel := context.WithCancel(context.Background())
		_, err := ssaapi.ParseProjectWithFS(vf,
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programID),
			ssaapi.WithProcess(func(msg string, process float64) {
				if process >= 0.5 {
					cancel()
				}
				if process > maxProcess {
					maxProcess = process
				}
				log.Infof("message %v, process: %f", msg, process)
			}),
			ssaapi.WithContext(ctx),
		)
		require.Error(t, err, "parse project error: %v", err)
		// when cancel, the process will not 1
		require.LessOrEqual(t, maxProcess, 0.9)
		file := make([]string, 0)
		dbfs := ssadb.NewIrSourceFs()
		filesys.Recursive(
			fmt.Sprintf("/%s", programID),
			filesys.WithFileSystem(dbfs),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				_, path, _ := strings.Cut(s, programID+"/")
				file = append(file, path)
				return nil
			}),
		)
		require.LessOrEqual(t, len(file), 10)
	})

}

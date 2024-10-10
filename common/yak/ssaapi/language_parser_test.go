package ssaapi_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func Test_CompileError(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a.yak", "print('Hello,')")
	vf.AddFile("b.yak", "print('Hello, ")
	vf.AddFile("c.yak", "print('Hello,')")

	t.Run("test compile error with fast fail ", func(t *testing.T) {
		// Compile the file
		progName := uuid.NewString()
		finalProcess := 0.0
		prog, err := ssaapi.ParseProject(vf,
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

		irProg, err := ssadb.GetProgram(progName, string(ssa.Application))
		require.Nil(t, irProg)
		require.Error(t, err)
		require.True(t, gorm.IsRecordNotFoundError(err))

	})

	t.Run("test compile error without fast fail  ", func(t *testing.T) {
		finalProcess := 0.0
		prog, err := ssaapi.ParseProject(vf, ssaapi.WithProcess(func(msg string, process float64) {
			if process > finalProcess {
				finalProcess = process
			}
		}))
		require.NoError(t, err)
		_ = prog
		require.Equal(t, finalProcess, 1.0)
	})

}

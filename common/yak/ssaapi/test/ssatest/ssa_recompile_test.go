package ssatest

import (
	"fmt"
	"io/fs"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestJarRecompile(t *testing.T) {
	jarPath, err := GetJarFile()
	require.NoError(t, err)

	// compile
	progName := uuid.NewString()
	res, err := ssaapi.ParseProject(
		ssaapi.WithRawLanguage("java"),
		ssaapi.WithConfigInfo(map[string]any{
			"kind":       "compression",
			"local_file": jarPath,
		}),
		ssaapi.WithProgramName(progName),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)

	}()
	require.NoErrorf(t, err, "error: %v", err)
	require.NotNil(t, res)

	// check program list
	fileList := make([]string, 0)
	filesys.Recursive(
		fmt.Sprintf("/%s", progName),
		filesys.WithFileSystem(ssadb.NewIrSourceFs()),
		filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
			fileList = append(fileList, s)
			return nil
		}),
	)
	require.Greater(t, len(fileList), 0)
	log.Infof("file list: %v", fileList)

	// check info in ssa-program
	prog, err := ssadb.GetProgram(progName, ssadb.Application)
	require.NoError(t, err)
	require.NotNil(t, prog)
	log.Infof("config input: %v", prog)
	require.True(t, len(prog.ConfigInput) > 0)

	// load from database
	progFromDB, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.NotNil(t, progFromDB)

	// recompile
	hasProcess := false
	finish := false
	log.Errorf("re compile")
	err = progFromDB.Recompile(ssaapi.WithProcess(func(msg string, process float64) {
		if 0 < process && process < 1 {
			hasProcess = true
		}

		if process == 1 {
			finish = true
		}
	}))
	require.NoError(t, err)
	require.True(t, hasProcess)
	require.True(t, finish)

	// check program list
	fileList = make([]string, 0)
	filesys.Recursive(
		fmt.Sprintf("/%s", progName),
		filesys.WithFileSystem(ssadb.NewIrSourceFs()),
		filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
			fileList = append(fileList, s)
			return nil
		}),
	)
	require.Greater(t, len(fileList), 0)
	log.Infof("file list: %v", fileList)

}

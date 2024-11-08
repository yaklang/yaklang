package ssatest

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

//go:embed java-realworld.zip
var javazip embed.FS

func Test_Multiple_input(t *testing.T) {
	t.Run("test multiple input", func(t *testing.T) {
		// write java zip file to template directory
		dir := os.TempDir()
		zipData, err := javazip.ReadFile("java-realworld.zip")
		require.NoError(t, err)

		zipPath := dir + "/java-realworld.zip"
		err = os.WriteFile(zipPath, zipData, 0644)
		require.NoError(t, err)

		info := `
		{
			"kind": "compression",
			"local_file": "` + zipPath + `"
		}
		`
		progName := uuid.NewString()
		res, err := ssaapi.ParseProject(
			ssaapi.WithLanguage(ssaapi.JAVA),
			ssaapi.WithConfigInfo(info),
			ssaapi.WithProgramName(progName),
			ssaapi.WithSaveToProfile(),
		)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progName)
			ssadb.DeleteSSAProgram(progName)
		}()
		require.NoErrorf(t, err, "error: %v", err)
		require.NotNil(t, res)

		fileList := make([]string, 0)
		filesys.Recursive(
			fmt.Sprintf("/%s", progName),
			filesys.WithFileSystem(ssadb.NewIrSourceFs()),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				fileList = append(fileList, s)
				return nil
			}),
		)

		// in ssa-program
		ssaprog := ssadb.CheckAndSwitchDB(progName)
		require.NotNil(t, ssaprog)
		log.Infof("config input: %v", ssaprog)
		require.True(t, len(ssaprog.ConfigInput) > 0)

		require.Greater(t, len(fileList), 0)
		log.Infof("file list: %v", fileList)
	})
}

package ssatest

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/javaclassparser"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestJar(t *testing.T) {
	jarPath, err := GetJarFile()
	require.NoError(t, err)
	// test jar filesystem
	jarFs, err := javaclassparser.NewJarFSFromLocal(jarPath)
	require.NoError(t, err)

	t.Run("test jar walk", func(t *testing.T) {
		fileList := make([]string, 0)
		filesys.Recursive(
			".",
			filesys.WithFileSystem(jarFs),
			filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
				log.Infof("isDir: %v, pathname: %v", isDir, pathname)
				if isDir {
					return nil
				}
				fileList = append(fileList, pathname)

				data, err := jarFs.ReadFile(pathname)
				if err != nil {
					log.Errorf("read file %s failed: %v", pathname, err)
					return err
				}
				log.Info(string(data))
				return nil
			}),
		)
		require.True(t, len(fileList) > 0)

	})

	t.Run("test jar compile", func(t *testing.T) {
		progName := uuid.NewString()
		prog, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(jarFs),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(progName),
		)
		require.NoError(t, err)
		require.NotNil(t, prog)

		fileList := make([]string, 0)
		filesys.Recursive(
			fmt.Sprintf("/%s", progName),
			filesys.WithFileSystem(ssadb.NewIrSourceFs()),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				fileList = append(fileList, s)
				return nil
			}),
		)
		log.Infof("file list: %v", fileList)
		require.Greater(t, len(fileList), 0)
	})
}
func checkFilelist(t *testing.T, language string, info map[string]any) {
	progName := uuid.NewString()
	res, err := ssaapi.ParseProject(
		ssaapi.WithRawLanguage(language),
		ssaapi.WithConfigInfo(info),
		ssaapi.WithProgramName(progName),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
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
	require.Greater(t, len(fileList), 0)
	log.Infof("file list: %v", fileList)

	// in ssa-program
	prog, err := ssadb.GetProgram(progName, ssadb.Application)
	require.NoError(t, err)
	require.NotNil(t, prog)
	log.Infof("config input: %v", prog)
	require.True(t, len(prog.ConfigInput) > 0)

	progDB, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.NotNil(t, progDB)
}

func Test_Multiple_input(t *testing.T) {

	t.Run("test compression input", func(t *testing.T) {
		// write java zip file to template directory
		zipPath, err := GetZipFile()
		require.NoError(t, err)

		checkFilelist(t, "java", map[string]any{
			"kind":       "compression",
			"local_file": zipPath,
		})
	})

	t.Run("test jar input", func(t *testing.T) {
		jarPath, err := GetJarFile()
		require.NoError(t, err)

		checkFilelist(t, "java", map[string]any{
			"kind":       "jar",
			"local_file": jarPath,
		})
	})

}

func Test_Multiple_input_git(t *testing.T) {
	url, err := GetLocalGit()
	require.NoError(t, err)

	t.Run("test git clone", func(t *testing.T) {
		targetPath := path.Join(os.TempDir(), "java-real")
		os.RemoveAll(targetPath)
		os.Mkdir(targetPath, 0755)
		err := yakgit.Clone(url, targetPath)
		require.NoError(t, err)

		refFs := filesys.NewRelLocalFs(targetPath)
		fileLen := 0
		filesys.Recursive(".",
			filesys.WithFileSystem(refFs),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				log.Infof("file: %s:\n", s)
				content, err := refFs.ReadFile(s)
				_ = content
				require.NoError(t, err)
				// log.Infof("%s\n", string(content))
				fileLen++
				return nil
			}),
		)
		require.Greater(t, fileLen, 0)
	})

	t.Run("test ssa compile", func(t *testing.T) {
		checkFilelist(t, "java", map[string]any{
			"kind": "git",
			"url":  url,
		})
	})
}

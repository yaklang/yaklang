package ssatest

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
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
		ssaconfig.WithCodeSourceMap(info),
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

	t.Run("test zip with nested jar", func(t *testing.T) {
		zipPath, err := GetZipWithJarFile()
		require.NoError(t, err)

		zipFS, err := filesys.NewZipFSFromLocal(zipPath)
		require.NoError(t, err)

		expandedFS := javaclassparser.NewExpandedZipFS(zipFS, zipFS)

		jarPath := "lib/test.jar"
		jarFS, err := expandedFS.GetJarFS(jarPath)
		require.NoError(t, err)
		require.NotNil(t, jarFS)

		fileList := make([]string, 0)
		filesys.Recursive(
			".",
			filesys.WithFileSystem(jarFS),
			filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
				if !isDir {
					fileList = append(fileList, pathname)
					data, err := jarFS.ReadFile(pathname)
					if err != nil {
						log.Errorf("read file %s failed: %v", pathname, err)
						return err
					}
					require.NotEmpty(t, data, "file %s should have content", pathname)
				}
				return nil
			}),
		)
		require.Greater(t, len(fileList), 0, "should find files in nested jar")

		checkFilelist(t, "java", map[string]any{
			"kind":       "compression",
			"local_file": zipPath,
		})
	})

	t.Run("test zip with nested jar directory detection", func(t *testing.T) {
		zipPath, err := GetZipWithJarFile()
		require.NoError(t, err)

		zipFS, err := filesys.NewZipFSFromLocal(zipPath)
		require.NoError(t, err)

		expandedFS := javaclassparser.NewExpandedZipFS(zipFS, zipFS)

		dirs := make([]string, 0)
		files := make([]string, 0)

		filesys.Recursive(
			".",
			filesys.WithFileSystem(expandedFS),
			filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
				if isDir {
					dirs = append(dirs, pathname)
				} else {
					files = append(files, pathname)
				}
				return nil
			}),
		)

		log.Infof("directories found: %v", dirs)
		log.Infof("files found: %v", files)

		require.Greater(t, len(dirs), 0, "should find directories")
		require.Greater(t, len(files), 0, "should find files")

		hasJarDir := false
		for _, dir := range dirs {
			if strings.Contains(dir, ".jar") || strings.Contains(dir, ".zip") {
				hasJarDir = true
				log.Infof("found archive directory: %s", dir)
			}
		}
		require.True(t, hasJarDir, "should find jar/zip directories: dirs=%v", dirs)

		hasMainClass := false
		mainClassPath := ""
		for _, file := range files {
			if strings.Contains(file, "Main.java") || strings.HasSuffix(file, "Main.java") {
				hasMainClass = true
				mainClassPath = file
				log.Infof("found Main.java: %s", file)
				break
			}
		}
		require.True(t, hasMainClass, "should find Main.java file: files=%v", files)

		data, err := expandedFS.ReadFile(mainClassPath)
		require.NoError(t, err, "should be able to read Main.java: %s", mainClassPath)
		require.NotEmpty(t, data, "Main.java should have content")
		log.Infof("successfully read Main.java from %s, size: %d bytes", mainClassPath, len(data))
	})

}

func TestExpandedZipFS_JarMarkedAsDirectory_Compile(t *testing.T) {
	zipPath, err := GetZipWithJarFile()
	require.NoError(t, err)

	zipFS, err := filesys.NewZipFSFromLocal(zipPath)
	require.NoError(t, err)
	defer zipFS.Close()

	expandedFS := javaclassparser.NewExpandedZipFS(zipFS, zipFS)

	t.Run("jar file should be marked as directory in Stat", func(t *testing.T) {
		jarPath := "lib/test.jar"
		info, err := expandedFS.Stat(jarPath)
		require.NoError(t, err)
		require.NotNil(t, info)
		require.True(t, info.IsDir(), "jar file should be marked as directory")
		log.Infof("jar file %s is marked as directory: %v", jarPath, info.IsDir())
	})

	t.Run("compile with parseProjectWithFS should handle jar as directory", func(t *testing.T) {
		progName := uuid.NewString()
		prog, err := ssaapi.ParseProjectWithFS(
			expandedFS,
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(progName),
		)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progName)
		}()

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

		require.Greater(t, len(fileList), 0, "should find files after compilation")
		log.Infof("compiled file list: %v", fileList)

		hasMainJava := false
		for _, file := range fileList {
			if strings.Contains(file, "Main.java") {
				hasMainJava = true
				log.Infof("found Main.java in compiled files: %s", file)
				break
			}
		}
		require.True(t, hasMainJava, "should find Main.java from nested jar after compilation")
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

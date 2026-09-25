package ssaapi_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// programFSConfig is the shared compile/list setup for ProgramFileSystem cases.
type programFSConfig struct {
	name  string
	files map[string]string
	opts  []ssaconfig.Option
}

func TestProgramFileSystem(t *testing.T) {
	t.Run("full_compile", func(t *testing.T) {
		programID := "prog_" + uuid.NewString()
		cfg := programFSConfig{
			name: programID,
			files: map[string]string{
				"src/A.java": `package src; class A { void m(){} }`,
				"src/B.java": `package src; class B { void m(){} }`,
			},
			opts: []ssaconfig.Option{
				ssaapi.WithLanguage(ssaconfig.JAVA),
				ssaapi.WithProgramName(programID),
			},
		}
		_, err := ssaconfig.New(ssaconfig.ModeProjectCompile, cfg.opts...)
		require.NoError(t, err)

		vf := filesys.NewVirtualFs()
		for path, code := range cfg.files {
			vf.AddFile(path, code)
		}
		_, err = ssaapi.ParseProjectWithFS(vf, cfg.opts...)
		require.NoError(t, err)
		t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), programID) })

		root := "/" + programID
		progFS := ssaapi.NewProgramFileSystem()
		dbFS := ssadb.NewIrSourceFs()

		progFiles := collectProgramFiles(t, progFS, root)
		dbFiles := collectProgramFiles(t, dbFS, root)
		require.True(t, hasProgramFSFileBySuffix(progFiles, "A.java"))
		require.True(t, hasProgramFSFileBySuffix(progFiles, "B.java"))
		require.Equal(t, len(progFiles), len(dbFiles),
			"full compile: ProgramFileSystem and IrSourceFS must see the same tree")

		first, err := progFS.ReadDir(root)
		require.NoError(t, err)
		require.NotEmpty(t, first)
		for i := 0; i < 20; i++ {
			again, err := progFS.ReadDir(root)
			require.NoError(t, err)
			require.Equal(t, len(first), len(again))
		}

		data, err := readProgramFileBySuffix(progFS, progFiles, "A.java")
		require.NoError(t, err)
		require.Contains(t, string(data), "class A")
	})

	t.Run("overlay_aggregate", func(t *testing.T) {
		baseOpts := []ssaconfig.Option{
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithEnableIncrementalCompile(true),
			ssaapi.WithContext(context.Background()),
		}
		_, err := ssaconfig.New(ssaconfig.ModeProjectCompile, baseOpts...)
		require.NoError(t, err)

		ssatest.CheckIncrementalProgramWithOptions(t, baseOpts,
			ssatest.IncrementalStep{
				Files: map[string]string{
					"A.java": `
public class A {
  public String getValue() {
    return "base";
  }
}`,
					"Utils.java": `
public class Utils {
  public static String id() { return "utils"; }
}`,
				},
				Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
					if stage != ssatest.IncrementalCheckStageDB {
						return
					}
					require.NotNil(t, overlay)
				},
			},
			ssatest.IncrementalStep{
				Files: map[string]string{
					"A.java": `
public class A {
  public String getValue() {
    return "diff";
  }
}`,
					"New.java": `
public class New {
  public void neu() {}
}`,
				},
				Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
					if stage != ssatest.IncrementalCheckStageDB {
						return
					}
					require.NotNil(t, overlay)
					require.GreaterOrEqual(t, overlay.ProgramCount(), 2)

					names := overlay.ProgramNames()
					top := names[len(names)-1]
					root := "/" + top

					progFS := ssaapi.NewProgramFileSystem()
					fileSet := collectProgramFiles(t, progFS, root)
					require.True(t, hasProgramFSFileBySuffix(fileSet, "A.java"))
					require.True(t, hasProgramFSFileBySuffix(fileSet, "Utils.java"),
						"aggregated tree must keep untouched base file Utils.java")
					require.True(t, hasProgramFSFileBySuffix(fileSet, "New.java"))

					data, err := readProgramFileBySuffix(progFS, fileSet, "A.java")
					require.NoError(t, err)
					require.Contains(t, string(data), "diff")

					dbFS := ssadb.NewIrSourceFs()
					dbFiles := collectProgramFiles(t, dbFS, root)
					require.False(t, hasProgramFSFileBySuffix(dbFiles, "Utils.java"),
						"ssadb IrSourceFS must not aggregate overlay layers")
				},
			},
		)
	})
}

func collectProgramFiles(t *testing.T, fsys fi.FileSystem, root string) map[string]bool {
	t.Helper()
	fileSet := make(map[string]bool)
	err := filesys.Recursive(root, filesys.WithFileSystem(fsys), filesys.WithFileStat(func(p string, info fs.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		fileSet[p] = true
		return nil
	}))
	require.NoError(t, err)
	return fileSet
}

func readProgramFileBySuffix(fsys fi.FileSystem, fileSet map[string]bool, name string) ([]byte, error) {
	for p := range fileSet {
		if p == name || strings.HasSuffix(p, "/"+name) {
			return fsys.ReadFile(p)
		}
	}
	return fsys.ReadFile(name)
}

func hasProgramFSFileBySuffix(fileSet map[string]bool, name string) bool {
	for path := range fileSet {
		if path == name || strings.HasSuffix(path, "/"+name) {
			return true
		}
	}
	return false
}

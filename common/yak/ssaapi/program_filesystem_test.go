package ssaapi_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestProgramFileSystem_OverlayAggregatesOutsideSSADB(t *testing.T) {
	ssatest.CheckIncrementalProgram(t,
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

				// ssadb only hydrates this program's ir_sources rows. Untouched
				// base files live on the base program, so Utils.java must not appear.
				dbFS := ssadb.NewIrSourceFs()
				dbFiles := collectProgramFiles(t, dbFS, root)
				require.False(t, hasProgramFSFileBySuffix(dbFiles, "Utils.java"),
					"ssadb IrSourceFS must not aggregate overlay layers")
			},
		},
	)
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

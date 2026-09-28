package ssaapi_test

import (
	"context"
	"io/fs"
	"sort"
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
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

	t.Run("diff_only", func(t *testing.T) {
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
					"Base.java": `
public class Base {
  public void b() {}
}`,
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

					// Last-diff-only view lists exactly the diff layer files.
					diffFiles := collectProgramFiles(t, diffOnlyViewFS{progFS}, root)
					require.True(t, hasProgramFSFileBySuffix(diffFiles, "A.java"))
					require.True(t, hasProgramFSFileBySuffix(diffFiles, "New.java"))
					require.False(t, hasProgramFSFileBySuffix(diffFiles, "Utils.java"),
						"diff-only view must not list untouched base files")
					require.False(t, hasProgramFSFileBySuffix(diffFiles, "Base.java"))

				// Aggregate view keeps untouched base files.
				aggFiles := collectProgramFiles(t, progFS, root)
					require.True(t, hasProgramFSFileBySuffix(aggFiles, "Utils.java"))
					require.True(t, hasProgramFSFileBySuffix(aggFiles, "New.java"))

					// Root program listing is not affected by diffOnly.
					diffRoot, err := progFS.ReadDirDiffOnly("/")
					require.NoError(t, err)
					aggRoot, err := progFS.ReadDir("/")
					require.NoError(t, err)
					require.Equal(t, dirEntryNames(aggRoot), dirEntryNames(diffRoot))

					// ReadFile still resolves through the aggregate (superset),
					// so jumping into base files from audit results keeps working.
					utilsPath := findProgramFSPathBySuffix(aggFiles, "Utils.java")
					require.NotEmpty(t, utilsPath)
					data, err := progFS.ReadFile(utilsPath)
					require.NoError(t, err)
					require.Contains(t, string(data), "utils")

					// YakURL op=list honors diffOnly=true.
					diffList := yakurlListProgramDir(t, root, &ypb.KVPair{Key: "diffOnly", Value: "true"})
					require.Contains(t, diffList, root+"/A.java")
					require.Contains(t, diffList, root+"/New.java")
					require.NotContains(t, diffList, root+"/Utils.java")
					aggList := yakurlListProgramDir(t, root)
					require.Contains(t, aggList, root+"/Utils.java")

					// ExtraInfo exposes incrementality so clients know whether
					// the diffOnly view is meaningful for this program.
					extra := progFS.ExtraInfo(root)
					require.Equal(t, true, extra["IsIncremental"])
					// Base (full compile) programs are not incremental.
					baseExtra := progFS.ExtraInfo("/" + names[0])
					require.NotEqual(t, true, baseExtra["IsIncremental"])
				},
			},
			ssatest.IncrementalStep{
				// delete-only diff: nothing compiled, only Base.java removed
				Files: map[string]string{"Base.java": ""},
				Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
					if stage != ssatest.IncrementalCheckStageDB {
						return
					}
					names := overlay.ProgramNames()
					top := names[len(names)-1]
					root := "/" + top

					progFS := ssaapi.NewProgramFileSystem()
					entries, err := progFS.ReadDirDiffOnly(root)
					require.NoError(t, err, "delete-only diff must yield an empty tree, not an error")
					require.Empty(t, entries)

					aggFiles := collectProgramFiles(t, progFS, root)
					require.False(t, hasProgramFSFileBySuffix(aggFiles, "Base.java"),
						"aggregate view must apply deletions")
					require.True(t, hasProgramFSFileBySuffix(aggFiles, "A.java"))
				},
			},
		)
	})

	t.Run("diff_only_non_incremental", func(t *testing.T) {
		programID := "prog_" + uuid.NewString()
		opts := []ssaconfig.Option{
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programID),
		}
		_, err := ssaconfig.New(ssaconfig.ModeProjectCompile, opts...)
		require.NoError(t, err)

		vf := filesys.NewVirtualFs()
		vf.AddFile("src/A.java", `package src; class A { void m(){} }`)
		vf.AddFile("src/B.java", `package src; class B { void m(){} }`)
		_, err = ssaapi.ParseProjectWithFS(vf, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), programID) })

		root := "/" + programID
		progFS := ssaapi.NewProgramFileSystem()

		aggFiles := collectProgramFiles(t, progFS, root)
		diffFiles := collectProgramFiles(t, diffOnlyViewFS{progFS}, root)
		require.Equal(t, aggFiles, diffFiles,
			"non-incremental program: diff-only view must fall back to the regular view")
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

func findProgramFSPathBySuffix(fileSet map[string]bool, name string) string {
	for path := range fileSet {
		if path == name || strings.HasSuffix(path, "/"+name) {
			return path
		}
	}
	return ""
}

func dirEntryNames(entries []fs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// diffOnlyViewFS routes ReadDir through ProgramFileSystem.ReadDirDiffOnly so
// filesys.Recursive walks the last-diff-only view; everything else (Stat,
// ReadFile, ...) keeps the aggregate behavior of ProgramFileSystem.
type diffOnlyViewFS struct {
	*ssaapi.ProgramFileSystem
}

func (d diffOnlyViewFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return d.ProgramFileSystem.ReadDirDiffOnly(name)
}

// yakurlListProgramDir lists path through the ssadb:// YakURL action,
// appending extraQuery to the default op=list.
func yakurlListProgramDir(t *testing.T, path string, extraQuery ...*ypb.KVPair) []string {
	t.Helper()
	local, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)
	res, err := local.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema: "ssadb",
			Path:   path,
			Query:  append([]*ypb.KVPair{{Key: "op", Value: "list"}}, extraQuery...),
		},
	})
	require.NoError(t, err)
	paths := make([]string, 0, len(res.Resources))
	for _, r := range res.Resources {
		paths = append(paths, r.Path)
	}
	return paths
}

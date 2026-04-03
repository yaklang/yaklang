package test

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed all:syntax/***
var projectSyntaxFS embed.FS

const pythonSyntaxFixtureRoot = "syntax"

type pythonSyntaxFixtureMode string

const (
	pythonSyntaxFixtureASTOnly pythonSyntaxFixtureMode = "ast_only"
	pythonSyntaxFixtureCompile pythonSyntaxFixtureMode = "compile"
)

var pythonSyntaxFixtureRootModes = map[string]pythonSyntaxFixtureMode{
	"django-filer":      pythonSyntaxFixtureCompile,
	"django_filer_meta": pythonSyntaxFixtureCompile,
	"frappe":            pythonSyntaxFixtureCompile,
	"g4":                pythonSyntaxFixtureASTOnly,
	"lektor":            pythonSyntaxFixtureCompile,
	"quokka":            pythonSyntaxFixtureCompile,
	"tutor":             pythonSyntaxFixtureCompile,
	"wagtail":           pythonSyntaxFixtureCompile,
	"wagtail_nested":    pythonSyntaxFixtureCompile,
}

func collectPythonSyntaxFixtureFiles(fsys fs.FS, root string) ([]string, error) {
	files := make([]string, 0)
	err := fs.WalkDir(fsys, root, func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(filePath, ".py") {
			return nil
		}
		files = append(files, filePath)
		return nil
	})
	sort.Strings(files)
	return files, err
}

func collectPythonSyntaxFixtureFilesFromDisk(root string) ([]string, error) {
	return collectPythonSyntaxFixtureFiles(os.DirFS("."), root)
}

func collectPythonSyntaxFixtureRoots(fsys fs.FS, root string) ([]string, error) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, err
	}

	roots := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			roots = append(roots, entry.Name())
		}
	}
	sort.Strings(roots)
	return roots, nil
}

func TestPythonProjectSyntaxFixturesCompile(t *testing.T) {
	roots, err := collectPythonSyntaxFixtureRoots(projectSyntaxFS, pythonSyntaxFixtureRoot)
	require.NoError(t, err)
	require.NotEmpty(t, roots, "no embedded python syntax fixture roots found")
	require.Len(t, pythonSyntaxFixtureRootModes, len(roots), "every syntax root must be explicitly classified")

	projectRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		mode, ok := pythonSyntaxFixtureRootModes[root]
		require.Truef(t, ok, "syntax root %q is not classified", root)
		if mode == pythonSyntaxFixtureCompile {
			projectRoots = append(projectRoots, path.Join(pythonSyntaxFixtureRoot, root))
		}
	}
	require.NotEmpty(t, projectRoots, "no compile-mode python syntax fixtures found")

	for _, root := range projectRoots {
		t.Run(strings.TrimPrefix(root, "syntax/"), func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			files, err := collectPythonSyntaxFixtureFiles(projectSyntaxFS, root)
			require.NoError(t, err)
			require.NotEmpty(t, files, "compile fixture root %q has no python files", root)
			for _, filePath := range files {
				raw, err := projectSyntaxFS.ReadFile(filePath)
				require.NoError(t, err)
				vf.AddFile(strings.TrimPrefix(filePath, root+"/"), string(raw))
			}

			progs, err := ssaapi.ParseProjectWithFS(
				vf,
				ssaapi.WithLanguage(ssaconfig.PYTHON),
				ssaapi.WithMemory(true),
			)
			require.NoError(t, err)
			require.NotEmpty(t, progs)
			for _, prog := range progs {
				require.Len(t, prog.GetErrors(), 0, prog.GetErrors().String())
			}
		})
	}
}

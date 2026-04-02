package test

import (
	"embed"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed syntax/***
var projectSyntaxFS embed.FS

func TestAllEmbeddedProjectSyntaxForPython_G4(t *testing.T) {
	err := fs.WalkDir(projectSyntaxFS, "syntax", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() || !strings.HasSuffix(filePath, ".py") {
			return nil
		}
		raw, err := projectSyntaxFS.ReadFile(filePath)
		require.NoError(t, err)
		validateSource(t, filePath, string(raw))
		return nil
	})
	require.NoError(t, err)
}

func TestPythonProjectSyntaxFixturesCompile(t *testing.T) {
	entries, err := projectSyntaxFS.ReadDir("syntax")
	require.NoError(t, err)

	projectRoots := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			projectRoots = append(projectRoots, path.Join("syntax", entry.Name()))
		}
	}
	sort.Strings(projectRoots)
	require.NotEmpty(t, projectRoots, "no embedded python project fixtures found")

	for _, root := range projectRoots {
		t.Run(strings.TrimPrefix(root, "syntax/"), func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			err := fs.WalkDir(projectSyntaxFS, root, func(filePath string, d fs.DirEntry, walkErr error) error {
				require.NoError(t, walkErr)
				if d.IsDir() || !strings.HasSuffix(filePath, ".py") {
					return nil
				}
				raw, err := projectSyntaxFS.ReadFile(filePath)
				require.NoError(t, err)
				vf.AddFile(strings.TrimPrefix(filePath, root+"/"), string(raw))
				return nil
			})
			require.NoError(t, err)

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

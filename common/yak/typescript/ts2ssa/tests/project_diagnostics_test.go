package tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/typescript/ts2ssa"
)

func TestProjectDiagnosticsAreNotReplicated(t *testing.T) {
	for _, language := range []ssaconfig.Language{ssaconfig.JS, ssaconfig.TS} {
		for _, workers := range []int{1, 4} {
			for _, tc := range []struct {
				name    string
				code    string
				message string
				perFile int
			}{
				// The declaration is checked during both prehandling and body build.
				{"declaration", "const missing;", ts2ssa.ConstDeclarationWithoutInitializer(), 2},
				{"body", "break;", ts2ssa.UnexpectedBreakStmt(), 1},
			} {
				t.Run(fmt.Sprintf("%s/workers=%d/%s", language, workers, tc.name), func(t *testing.T) {
					const fileCount = 4
					fs := filesys.NewVirtualFs()
					for i := 0; i < fileCount; i++ {
						fs.AddFile(fmt.Sprintf("unit%d/file%d.%s", i, i, language), tc.code)
					}
					programs, err := ssaapi.ParseProjectWithFS(fs,
						ssaapi.WithLanguage(language), ssaapi.WithMemory(true),
						ssaconfig.WithCompileConcurrency(workers),
					)
					require.NoError(t, err)
					require.Len(t, programs, 1)

					seen := make(map[*ssa.SSAError]bool)
					counts := make(map[string]int)
					for _, diagnostic := range programs[0].GetErrors() {
						require.NotNil(t, diagnostic)
						require.False(t, seen[diagnostic], "a diagnostic was appended to the application more than once")
						seen[diagnostic] = true
						if diagnostic.Tag == ts2ssa.TAG && diagnostic.Message == tc.message {
							require.NotNil(t, diagnostic.Pos)
							counts[diagnostic.Pos.GetEditor().GetFilename()]++
						}
					}
					require.Len(t, counts, fileCount, "diagnostics from every file must reach the application")
					for i := 0; i < fileCount; i++ {
						require.Equal(t, tc.perFile, counts[fmt.Sprintf("file%d.%s", i, language)])
					}
				})
			}
		}
	}
}

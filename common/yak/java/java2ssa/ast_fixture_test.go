package java2ssa_test

import (
	"embed"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed test/syntax
var syntaxFS embed.FS

func TestSyntaxFixtures(t *testing.T) {
	found := false
	err := fs.WalkDir(syntaxFS, "test/syntax", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() || !strings.HasSuffix(filePath, ".java") {
			return nil
		}

		raw, err := syntaxFS.ReadFile(filePath)
		require.NoError(t, err)

		fixtureName := strings.TrimPrefix(filePath, "test/syntax/")
		t.Run(fixtureName, func(t *testing.T) {
			src := string(raw)

			_, err := java2ssa.Frontend(src)
			require.NoError(t, err, "frontend parse failed for %s", fixtureName)

			prog, err := ssaapi.Parse(
				src,
				ssaapi.WithLanguage(ssaconfig.JAVA),
				ssaapi.WithProgramName("java2ssa_ast_fixture_"+sanitizeFixtureProgramName(fixtureName)),
			)
			require.NoError(t, err, "ssa parse failed for %s", fixtureName)
			require.NotNil(t, prog)
		})

		found = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, found, "no java syntax fixtures found under test/syntax")
}

func sanitizeFixtureProgramName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ".", "_", "-", "_")
	return replacer.Replace(name)
}

package sfpattern

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/minirehs"
)

func TestPathIndexAgreesWithLegacyMatcher(t *testing.T) {
	exprs := []string{
		"*", "*.*", "*.php", "*.go", "file.txt", `.*\.php$`,
		"**/*.go", "foo.{js,ts}", "a+b", "src/*.go",
	}
	paths := []string{
		"a.php", "dir/a.php", "src/a.go", "src/nested/a.go", "file.txt",
		"foo.js", "foo.ts", "aXb", "a+b", "*.*", "noext", `dir\a.php`,
	}
	for _, expr := range exprs {
		legacy := compilePathMatcher(expr)
		idx := buildPathIndex(map[string]sourceSearch{
			"k": {path: expr, positive: []int{0}, matchPath: legacy},
		})
		var scratch minirehs.Scratch
		if idx.db != nil {
			var err error
			scratch, err = idx.db.NewScratch()
			require.NoError(t, err)
		}
		for _, file := range paths {
			allowed := idx.allow(file, scratch, 1)
			require.Equalf(t, legacy(file), len(allowed) == 1 && allowed[0], "expr %q path %q", expr, file)
		}
		if scratch != nil {
			require.NoError(t, scratch.Close())
		}
		require.NoError(t, idx.Close())
	}
}

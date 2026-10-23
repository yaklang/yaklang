package ssaapi

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Building a JS file transfers the library's errors onto the application. Sub
// programs share the application's error list, so the transfer must be a no-op;
// the old copy-back loop doubled the list for every built file and made a full
// coremail memory compile grow without bound until the OOM killer took it.
func TestJSCompileKeepsErrorListLinear(t *testing.T) {
	build := func(fileCount int) int {
		fs := filesys.NewVirtualFs()
		for i := 0; i < fileCount; i++ {
			fs.AddFile(fmt.Sprintf("js/m%d.js", i), fmt.Sprintf(`
const v%d;
exports.m%d = 1;
`, i, i))
		}
		programs, err := ParseProjectWithFS(fs, WithLanguage(ssaconfig.JS), WithMemory(),
			ssaconfig.WithCompileConcurrency(1))
		require.NoError(t, err)
		require.Len(t, programs, 1)
		return len(programs[0].GetErrors())
	}

	one := build(1)
	four := build(4)
	require.Greater(t, one, 0, "the fixture must produce at least one SSA error")
	// Linear growth allows at most a small constant per file. Any doubling
	// pattern (the old behavior) pushes 4 files past 8x the single-file count.
	require.LessOrEqual(t, four, one*4+8,
		"error list must grow linearly with the file count: one=%d four=%d", one, four)
}

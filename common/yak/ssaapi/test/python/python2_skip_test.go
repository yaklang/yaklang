package python

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// Strict mode used to fail on a project that carries a vendored Python 2 file:
// the parse error was recorded as a project AST error. The file must be a
// classified skip, and the Python 3 sources must still compile.
func TestStrictModeSkipsPython2File(t *testing.T) {
	yakit.InitialDatabase()

	vfs := filesys.NewVirtualFs()
	vfs.AddFile("legacy.py", "value = ur'legacy'\n")
	vfs.AddFile("modern.py", "def modern():\n    return 1\n")

	progName := uuid.NewString()
	progs, err := ssaapi.ParseProjectWithFS(vfs,
		ssaapi.WithLanguage(ssaconfig.PYTHON),
		ssaapi.WithStrictMode(true),
		ssaapi.WithProgramName(progName),
	)
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), progName) })
	require.NoError(t, err, "a Python 2 file must be skipped, not fail strict compile")
	require.NotEmpty(t, progs)

	result, err := progs[0].SyntaxFlowWithError(`modern as $f`)
	require.NoError(t, err)
	require.NotEmpty(t, result.GetValues("f"), "Python 3 sources must still compile")
}

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

// A .py file that is actually a Jinja template must be a classified skip.
// Strict mode used to fail the whole project on its parse error.
func TestStrictModeSkipsTemplateFile(t *testing.T) {
	yakit.InitialDatabase()

	vfs := filesys.NewVirtualFs()
	vfs.AddFile("settings.py", "{% include 'partials/common.py' %}\n")
	vfs.AddFile("modern.py", "def modern():\n    return 1\n")

	progName := uuid.NewString()
	progs, err := ssaapi.ParseProjectWithFS(vfs,
		ssaapi.WithLanguage(ssaconfig.PYTHON),
		ssaapi.WithStrictMode(true),
		ssaapi.WithProgramName(progName),
	)
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), progName) })
	require.NoError(t, err, "a template file must be skipped, not fail strict compile")
	require.NotEmpty(t, progs)

	result, err := progs[0].SyntaxFlowWithError(`modern as $f`)
	require.NoError(t, err)
	require.NotEmpty(t, result.GetValues("f"), "Python 3 sources must still compile")
}

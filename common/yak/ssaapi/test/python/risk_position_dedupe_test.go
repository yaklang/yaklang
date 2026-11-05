package python

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// A constant-range loop is unrolled into one IR call per iteration. Every
// iteration has the same source position, so the result must keep one risk
// row for that position instead of one row per unrolled call.
func TestRiskDedupeBySourcePosition(t *testing.T) {
	rule, err := os.ReadFile("../../../../syntaxflow/sfbuildin/buildin/python/cwe-94-code-injection/struct-python-exec.sf")
	require.NoError(t, err)

	vfs := filesys.NewVirtualFs()
	vfs.AddFile("loop.py", "for i in range(20):\n    exec(\"pass\")\n")

	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		// The in-memory pass has no program name; buildSSARisk needs one.
		for _, prog := range programs {
			if prog != nil && prog.Program != nil && prog.Program.Name == "" {
				prog.Program.Name = "risk-position-dedupe"
			}
		}
		result, err := programs.SyntaxFlowWithError(string(rule), ssaapi.QueryWithMemory())
		require.NoError(t, err)

		// The rule itself still reports every unrolled call...
		require.Len(t, result.GetValues("call"), 20, "fixture must actually unroll the call")

		// ...but one source position is one published risk.
		risks := result.GetRisks()
		require.Len(t, risks, 1, "unrolled calls at one position must publish one risk")
		require.Equal(t, int64(2), risks[0].Line)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PYTHON))
}

package tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestCSharp_DatabaseReload_PreservesOverloadAndRefOutDataFlow(t *testing.T) {
	programName := "csharp-reload-" + uuid.NewString()
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), programName) })

	_, err := ssaapi.Parse(`
public class ReloadHarness {
    public static object Pick(int value) { return reloadClean(); }
    public static object Pick(string value) { return reloadOverloadSource(); }
    public static void Fill(out object value) { value = reloadOutSource(); }
    public void Change(ref object value) { value = reloadRefSource(); }

    public static void Main() {
        reloadOverloadSink(Pick("x"));

        object outValue;
        Fill(out outValue);
        reloadOutSink(outValue);

        object refValue = reloadClean();
        new ReloadHarness().Change(ref refValue);
        reloadRefSink(refValue);
    }
}
`, ssaapi.WithLanguage(ssaconfig.CSHARP), ssaapi.WithProgramName(programName))
	require.NoError(t, err)

	prog, err := ssaapi.FromDatabase(programName)
	require.NoError(t, err)
	require.NotNil(t, prog)

	for _, item := range []struct {
		name   string
		sink   string
		source string
	}{
		{name: "selected overload", sink: "reloadOverloadSink", source: "reloadOverloadSource"},
		{name: "static out", sink: "reloadOutSink", source: "reloadOutSource"},
		{name: "instance ref", sink: "reloadRefSink", source: "reloadRefSource"},
	} {
		t.Run(item.name, func(t *testing.T) {
			flow, queryErr := prog.SyntaxFlowWithError(item.sink + `(* #-> as $origin)`)
			require.NoError(t, queryErr)
			require.Contains(t, flow.GetValues("origin").String(), item.source)
		})
	}
}

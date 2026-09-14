package test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// parsePythonProject compiles the virtual FS and returns the application
// program, failing the test on compile errors.
func parsePythonProject(t *testing.T, vf *filesys.VirtualFS) *ssaapi.Program {
	t.Helper()

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
	return progs[0]
}

// upstreamKeys lists the application UpStream cache keys for diagnostics.
func upstreamKeys(prog *ssaapi.Program) []string {
	return prog.Program.UpStream.Keys()
}

// upstreamFileHashKeys returns the UpStream keys that look like on-demand
// file-hash sub-programs (64-char hex from ensurePythonModuleBuilt) —
// virtual-library entries are dotted module names instead.
func upstreamFileHashKeys(prog *ssaapi.Program) []string {
	var keys []string
	for _, key := range prog.Program.UpStream.Keys() {
		if len(key) == 64 {
			keys = append(keys, key)
		}
	}
	return keys
}

// requireOnDemandCompiled asserts the on-demand import fallback compiled the
// given file: its pure source hash must appear in the application UpStream
// cache, which only the need-driven nested build (ensurePythonModuleBuilt)
// populates — the batch pipeline itself never registers file-hash
// sub-programs.
func requireOnDemandCompiled(t *testing.T, prog *ssaapi.Program, content string) {
	t.Helper()

	hash := codec.Sha256(content)
	require.Contains(t, upstreamFileHashKeys(prog), hash,
		"file must be compiled on demand by the import fallback: file-hash keys=%v all=%v",
		upstreamFileHashKeys(prog), upstreamKeys(prog))
}

// TestPythonImportFallback_CompilesModuleOnDemand covers the ordering gap no
// other mechanism bridges: main.py and settings.py share the root compile
// unit (a self edge is dropped, so the topological order cannot help), and
// the skeleton pass only predeclares def/class shells — a module-level
// variable export does not exist until settings.py's deferred file task
// runs. File tasks execute in registration order, so main.py's import runs
// first and binds a placeholder that would only be patched by
// fixImportCallback after the whole project finishes. The need-driven
// fallback instead compiles settings.py on demand right there: main.py sees
// the real string and the file's hash lands in the UpStream cache.
func TestPythonImportFallback_CompilesModuleOnDemand(t *testing.T) {
	settings := "SECRET = \"s3cret\"\n"
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", "from settings import SECRET\n")
	vf.AddFile("settings.py", settings)

	// Sanity: both files share the root unit, so no unit edge exists.
	edges := requirePyUnitEdges(t, vf)
	require.Empty(t, edges, "same-unit import must not produce unit edges")

	prog := parsePythonProject(t, vf)

	// Direct proof of the on-demand compile: the module file hash lives in
	// the application UpStream cache.
	requireOnDemandCompiled(t, prog, settings)

	// And the imported name binds the real value, not the Any placeholder
	// (which would only be replaced after the whole project finishes).
	res, err := prog.SyntaxFlowWithError(`SECRET as $secret`)
	require.NoError(t, err)
	secrets := res.GetValues("secret")
	require.NotEmpty(t, secrets, "SECRET from the on-demand-compiled module must bind")
	for _, v := range secrets {
		require.False(t, v.IsUndefined(),
			"SECRET must be the real value from the on-demand compile, got placeholder: %s", v.String())
	}
}

// TestPythonImportFallback_FunctionImportNeedsNoFallback is the control for
// the test above: a def export IS predeclared by the skeleton pass
// (predeclareTopLevelFuncShells publishes it before any deferred task runs),
// so the same-unit ordering gap never materializes for functions and the
// fallback has nothing to do.
func TestPythonImportFallback_FunctionImportNeedsNoFallback(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", "from helper import fn\n")
	vf.AddFile("helper.py", "def fn():\n    return 42\n")

	prog := parsePythonProject(t, vf)

	// No file compiled on demand — the skeleton export already satisfied the
	// import before the fallback could trigger.
	require.Empty(t, upstreamFileHashKeys(prog),
		"skeleton-predeclared function import must not trigger an on-demand compile: %v", upstreamKeys(prog))

	res, err := prog.SyntaxFlowWithError(`fn as $fn`)
	require.NoError(t, err)
	fns := res.GetValues("fn")
	require.NotEmpty(t, fns, "fn must bind")
	for _, v := range fns {
		require.False(t, v.IsUndefined(), "fn must bind the skeleton export: %s", v.String())
	}
}

// TestPythonImportFallback_CyclicImportsNoDeadlock verifies A imports B
// imports A terminates: the busy-set guard stops the recursion and the compile
// finishes without errors or hangs.
func TestPythonImportFallback_CyclicImportsNoDeadlock(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("cyc_a.py", `
from cyc_b import b_func

def a_func():
    return b_func()
`)
	vf.AddFile("cyc_b.py", `
from cyc_a import a_func

def b_func():
    return a_func()
`)

	done := make(chan *ssaapi.Program, 1)
	go func() {
		done <- parsePythonProject(t, vf)
	}()
	select {
	case prog := <-done:
		require.NotNil(t, prog)
	case <-time.After(60 * time.Second):
		t.Fatal("cyclic import compile deadlocked")
	}
}

// TestPythonImportFallback_ThirdPartyNameStaysPlaceholder verifies that an
// import whose module does not exist in the project (a third-party library
// like frappe) still compiles, still binds something usable, and never
// triggers an on-demand compile (there is no file to compile).
func TestPythonImportFallback_ThirdPartyNameStaysPlaceholder(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
from frappe import get_doc

doc = get_doc("User", "1")
`)

	prog := parsePythonProject(t, vf)

	// No file compiled on demand: the UpStream cache may hold virtual-library
	// entries (frappe), but no file-hash sub-programs.
	require.Empty(t, upstreamFileHashKeys(prog),
		"third-party import must not trigger an on-demand compile: %v", upstreamKeys(prog))

	// get_doc must exist as a binding (the Any placeholder) — member calls on
	// it must not crash the compile.
	res, err := prog.SyntaxFlowWithError(`get_doc as $doc`)
	require.NoError(t, err)
	require.NotEmpty(t, res.GetValues("doc"),
		"third-party import name must still bind (placeholder) after the fallback skips it")
	for _, v := range res.GetValues("doc") {
		require.False(t, strings.Contains(v.String(), "panic"),
			"placeholder value must be well-formed: %s", v.String())
	}
}
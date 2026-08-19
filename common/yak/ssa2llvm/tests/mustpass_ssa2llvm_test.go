package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak"
)

// mustpassSSA2LLVMScripts lists yaklang mustpass scripts that are verified to
// compile and run correctly under BOTH the yakvm interpreter (yak.Execute) and
// the ssa2llvm AOT compiler (real CLI compile + run). Each script is a
// self-contained top-level yak program that asserts its own invariants, so a
// non-zero exit or a runtime panic under ssa2llvm means the AOT path diverges
// from the interpreter.
//
// Only scripts with actual AOT verification evidence are listed:
//   - lowhttp_isresponse.yak: uses the poc module, which has a lightweight AOT
//     shim (PrunedShim) so the monolithic yaklib stays out of the AOT binary.
//   - defer-recover.yak: pure language defer/recover.
//   - json.yak / json-dumps.yak / json_dump_no_escape_html.yak: json module
//     AOT shim (loads/dumps/withIndent/noEscapeHTML).
//   - codec_decrypt.yak: codec AES-ECB exports with nil iv handling.
//
// Scripts that were NOT added (no passing AOT evidence):
//   - re2.yak / poc_replace_path_func.yak: depend on re2/str modules that do
//     not yet have lightweight AOT shims.
//   - buildin_len.yak: string slicing is not yet implemented by the AOT
//     backend.
//   - ssa.yak: ssa.YaklangScriptChecking result length assertion fails under
//     AOT (exit 255).
var mustpassSSA2LLVMScripts = []string{
	"lowhttp_isresponse.yak",
	"defer-recover.yak",
	"json.yak",
	"json-dumps.yak",
	"json_dump_no_escape_html.yak",
	"codec_decrypt.yak",
}

// TestMustPass_SSA2LLVM_DualRun compiles each supported mustpass script through
// the real shipping ssa2llvm CLI, runs the produced native binary under an
// empty environment (zero external runtime dependency), and asserts it exits 0
// with no runtime panic. It also runs the same script through the yakvm
// interpreter to confirm both backends agree.
func TestMustPass_SSA2LLVM_DualRun(t *testing.T) {
	repoRoot := RepoRoot(t)
	mpDir := filepath.Join(repoRoot, "common", "yak", "yaktest", "mustpass", "files")

	for _, name := range mustpassSSA2LLVMScripts {
		name := name
		t.Run(name, func(t *testing.T) {
			script := filepath.Join(mpDir, name)

			// 1. yakvm baseline: the script must pass under the interpreter.
			code, err := os.ReadFile(script)
			if err != nil {
				t.Fatalf("read %s: %v", script, err)
			}
			if _, err := yak.Execute(string(code), nil); err != nil {
				t.Fatalf("yakvm baseline failed for %s: %v", name, err)
			}

			// 2. ssa2llvm: compile via the real CLI and run the native binary.
			output := RunYakScriptFileWithCLI(t, script, nil)
			if strings.Contains(output, "panic") {
				t.Fatalf("ssa2llvm run of %s produced a runtime panic:\n%s", name, output)
			}
		})
	}
}

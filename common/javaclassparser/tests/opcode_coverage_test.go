package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)

// excludedOpcodes are the JVM opcodes deliberately left out of the 100% parse-coverage gate, each
// with the concrete reason it cannot be exercised by decompiling modern javac output. They are real
// opcodes the decompiler still has handlers for, but no class produced by a current toolchain (and
// none in our embedded corpus) emits them in a form that reaches the stack simulator.
var excludedOpcodes = map[int]string{
	core.OP_JSR:    "deprecated subroutine call; javac >= 6 never emits it and the JSR inliner removes it before stack simulation",
	core.OP_JSR_W:  "deprecated wide subroutine call; same as JSR, plus only used for >32KB branch offsets",
	core.OP_RET:    "deprecated subroutine return; paired with JSR, removed by the JSR inliner before stack simulation",
	core.OP_GOTO_W: "wide goto, only emitted when a branch offset exceeds 32KB; no corpus method is that large",
	core.OP_WIDE:   "operand-extension prefix; folded into the following opcode's IsWide flag, never dispatched standalone",
	core.OP_LDC_W:  "wide ldc, only emitted when the constant-pool index exceeds 255; no corpus class has that many constants",
	core.OP_NOP:    "javac never emits nop from compilable source (only bytecode rewriters/obfuscators do); the handler is a no-op return",
}

// knownParseOpcodes returns the set of real JVM opcodes the decompiler registers a handler for
// (opcode value in [0,201]; the synthetic OP_END/markers and the internal wide pseudo-instructions
// carry out-of-range or -1 opcodes and are filtered out).
func knownParseOpcodes() map[int]string {
	out := map[int]string{}
	for op, instr := range core.InstrInfos {
		if op < 0 || op > 201 || instr == nil {
			continue
		}
		out[op] = instr.Name
	}
	return out
}

// decompileClassFilesUnder walks dir, decompiles every *.class it finds (ignoring per-file errors:
// the decompiler's safety net never panics and records opcodes as it simulates the stack, which is
// all this coverage probe needs), and returns how many files it processed.
func decompileClassFilesUnder(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".class") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		func() {
			defer func() { _ = recover() }()
			_, _ = javaclassparser.Decompile(raw)
		}()
		count++
		return nil
	})
	return count
}

// compileAndDecompileBatteries compiles every testdata/codec/*.java battery with javac and decompiles
// the resulting top-level class, so the opcodes only the self-hosted algorithm batteries emit (long
// arithmetic, fcmp/dcmp, multianewarray, monitorenter/exit, ...) are counted too. No-op without javac.
func compileAndDecompileBatteries(t *testing.T) int {
	t.Helper()
	javac, err := exec.LookPath("javac")
	if err != nil {
		return 0
	}
	entries, err := os.ReadDir("testdata/codec")
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".java") {
			continue
		}
		className := strings.TrimSuffix(e.Name(), ".java")
		src, rerr := os.ReadFile(filepath.Join("testdata/codec", e.Name()))
		if rerr != nil {
			continue
		}
		dir := t.TempDir()
		srcPath := filepath.Join(dir, className+".java")
		if werr := os.WriteFile(srcPath, src, 0644); werr != nil {
			continue
		}
		if out, cerr := exec.Command(javac, "-encoding", "UTF-8", "-nowarn", "-d", dir, srcPath).CombinedOutput(); cerr != nil {
			t.Logf("javac failed for %s (skipping): %v\n%s", className, cerr, out)
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir, "codec", className+".class"))
		if rerr != nil {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			_, _ = javaclassparser.Decompile(raw)
		}()
		count++
	}
	return count
}

// TestOpcodeParseCoverage is the opcode parse-coverage gate: it decompiles the entire embedded class
// corpus (regression seeds + syntax-coverage classes) plus the self-hosted codec batteries, recording
// every opcode that reaches the stack simulator (calcOpcodeStackInfo). It then asserts that every
// real JVM opcode the decompiler registers a handler for is exercised, except the documented
// excludedOpcodes (deprecated/prefix/oversize-only forms). This guarantees the opcode parser is 100%
// covered by deterministic, CI-friendly inputs.
func TestOpcodeParseCoverage(t *testing.T) {
	known := knownParseOpcodes()

	core.EnableOpcodeHitRecording()
	files := decompileClassFilesUnder(t, ".")
	batteries := compileAndDecompileBatteries(t)
	core.DisableOpcodeHitRecording()
	hits := core.RecordedOpcodeHits()

	t.Logf("opcode coverage corpus: %d class files + %d compiled batteries; %d distinct opcodes hit",
		files, batteries, len(hits))

	required := map[int]string{}
	for op, name := range known {
		if _, ex := excludedOpcodes[op]; ex {
			continue
		}
		required[op] = name
	}

	var missing []string
	for op, name := range required {
		if hits[op] == 0 {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	covered := len(required) - len(missing)
	t.Logf("opcode parse coverage: %d/%d required opcodes (%.1f%%); %d documented exclusions",
		covered, len(required), 100*float64(covered)/float64(len(required)), len(excludedOpcodes))

	if len(missing) > 0 {
		t.Errorf("opcode parse coverage incomplete: %d required opcodes never reached calcOpcodeStackInfo: %s",
			len(missing), strings.Join(missing, ", "))
	}
}

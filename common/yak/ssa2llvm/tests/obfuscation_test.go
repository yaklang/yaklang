package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

func TestSSAObfuscationAddSub(t *testing.T) {
	code := `
check = () => {
	left = 40 + 2
	right = 50 - 8
	return left + right
}
`

	checkBinaryEx(t, code, "check", "yak", 84)
	checkBinaryExWithOptions(t, code, "check", "yak", 84, withCompileObfuscators("addsub"))
}

func TestLLVMObfuscationXOR(t *testing.T) {
	code := `
one = () => { return 40 }
two = () => { return 2 }
three = () => { return 50 }
four = () => { return 8 }
check = () => {
	left = one() + two()
	right = three() - four()
	return left + right
}
`

	checkBinaryEx(t, code, "check", "yak", 84)
	checkBinaryExWithOptions(t, code, "check", "yak", 84, withCompileObfuscators("xor"))
}

func TestHybridObfuscationCallRet(t *testing.T) {
	code := `
one = () => { return 40 }
two = () => { return 2 }
check = () => {
	return one() + two()
}
`

	checkBinaryEx(t, code, "check", "yak", 42)
	checkBinaryExWithOptions(t, code, "check", "yak", 42, withCompileObfuscators("callret"))
}

func TestHybridObfuscationCallRetFunctionChain(t *testing.T) {
	code := `
leaf = () => { return 7 }
mid = () => { return leaf() + 8 }
top = () => { return mid() + leaf() }
check = () => {
	return top() + mid()
}
`

	checkBinaryEx(t, code, "check", "yak", 37)
	checkBinaryExWithOptions(t, code, "check", "yak", 37, withCompileObfuscators("callret"))
}

func TestSSAObfuscationUnknownName(t *testing.T) {
	tmpIR := t.TempDir() + "/missing.ll"
	_, err := compiler.CompileToExecutable(
		compiler.WithCompileSourceCode(`check = () => { return 1 }`),
		compiler.WithCompileLanguage("yak"),
		compiler.WithCompileEmitLLVM(true),
		compiler.WithCompileOutputFile(tmpIR),
		compiler.WithCompileObfuscators("missing"),
	)
	if err == nil {
		t.Fatal("expected unknown obfuscator error")
	}
}

// --- Phase 3: New Layer 1 obfuscation passes ---

func TestLLVMObfuscationMBA(t *testing.T) {
	code := `
one = () => { return 40 }
two = () => { return 2 }
three = () => { return 50 }
four = () => { return 8 }
check = () => {
	left = one() + two()
	right = three() - four()
	return left + right
}
`
	checkBinaryEx(t, code, "check", "yak", 84)
	checkBinaryExWithOptions(t, code, "check", "yak", 84, withCompileObfuscators("mba"))
}

func TestLLVMObfuscationOpaque(t *testing.T) {
	code := `
one = () => { return 40 }
two = () => { return 2 }
check = () => {
	return one() + two()
}
`
	checkBinaryEx(t, code, "check", "yak", 42)
	checkBinaryExWithOptions(t, code, "check", "yak", 42, withCompileObfuscators("opaque"))
}

func TestLLVMObfuscationOpaqueWithBranch(t *testing.T) {
	code := `
check = () => {
	a = 40 + 2
	b = 50 - 8
	if a > b {
		return a
	}
	return b
}
`
	checkBinaryEx(t, code, "check", "yak", 42)
	checkBinaryExWithOptions(t, code, "check", "yak", 42, withCompileObfuscators("opaque"))
}

func TestLLVMObfuscationCombined(t *testing.T) {
	code := `
one = () => { return 40 }
two = () => { return 2 }
check = () => {
	return one() + two()
}
`
	// Test combining multiple obfuscation passes.
	checkBinaryExWithOptions(t, code, "check", "yak", 42, withCompileObfuscators("addsub", "xor"))
	checkBinaryExWithOptions(t, code, "check", "yak", 42, withCompileObfuscators("mba", "opaque"))
}

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

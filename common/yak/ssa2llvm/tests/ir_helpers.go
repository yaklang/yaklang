package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func MustParseLanguage(t *testing.T, raw string) ssaconfig.Language {
	t.Helper()

	language, err := ssaconfig.ValidateLanguage(raw)
	if err != nil {
		t.Fatalf("invalid language %q: %v", raw, err)
	}
	if language == "" {
		return ssaconfig.Yak
	}
	return language
}

func CompileFunctionSSAString(t *testing.T, code string, language string, entry string, apply func(*ssa.Program) error) string {
	t.Helper()

	progBundle, err := ssaapi.Parse(code, ssaapi.WithLanguage(MustParseLanguage(t, language)))
	if err != nil {
		t.Fatalf("parse ssa failed: %v", err)
	}
	if apply != nil {
		if err := apply(progBundle.Program); err != nil {
			t.Fatalf("apply ssa transform failed: %v", err)
		}
	}

	function := FindSSAFunction(t, progBundle.Program, entry)
	return strings.Join(CollectSSAInstructionLines(function), "\n")
}

func CompileLLVMIRString(t *testing.T, code string, language string, options ...compiler.CompileOption) string {
	t.Helper()

	tmpIR, err := os.CreateTemp("", "ssa2llvm-obf-*.ll")
	if err != nil {
		t.Fatalf("create temp llvm ir failed: %v", err)
	}
	_ = tmpIR.Close()
	defer os.Remove(tmpIR.Name())

	compileOptions := make([]compiler.CompileOption, 0, len(options)+4)
	compileOptions = append(compileOptions,
		compiler.WithCompileSourceCode(code),
		compiler.WithCompileLanguage(language),
		compiler.WithCompileEmitLLVM(true),
		compiler.WithCompileOutputFile(tmpIR.Name()),
	)
	compileOptions = append(compileOptions, options...)

	if _, err := compiler.CompileToExecutable(compileOptions...); err != nil {
		t.Fatalf("compile llvm ir failed: %v", err)
	}

	content, err := os.ReadFile(tmpIR.Name())
	if err != nil {
		t.Fatalf("read llvm ir failed: %v", err)
	}
	return string(content)
}

func FindSSAFunction(t *testing.T, program *ssa.Program, entry string) *ssa.Function {
	t.Helper()

	var target *ssa.Function
	program.EachFunction(func(candidate *ssa.Function) {
		if target == nil && candidate != nil && candidate.GetName() == entry {
			target = candidate
		}
	})
	if target == nil {
		t.Fatalf("function %q not found", entry)
	}
	return target
}

func CollectSSAInstructionLines(fn *ssa.Function) []string {
	var lines []string
	for _, blockID := range fn.Blocks {
		blockValue, ok := fn.GetValueById(blockID)
		if !ok || blockValue == nil {
			continue
		}
		block, ok := ssa.ToBasicBlock(blockValue)
		if !ok || block == nil {
			continue
		}
		for _, instID := range block.Insts {
			inst, ok := fn.GetInstructionById(instID)
			if !ok || inst == nil {
				continue
			}
			lines = append(lines, ssa.LineDisASM(inst))
		}
	}
	return lines
}

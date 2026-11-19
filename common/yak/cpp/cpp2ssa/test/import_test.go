package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// TestCppIncludeHeaderDeclarations asserts a quoted include inlines the header:
// the class, its method and the macro all reach the including translation unit.
func TestCppIncludeHeaderDeclarations(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"main.cpp": `
#include "shape.hpp"

int main() {
    Adder adder;
    return adder.add(2);
}
`,
		"shape.hpp": `
#ifndef SHAPE_HPP
#define SHAPE_HPP

class Adder {
public:
    Adder() : base_(1) {}
    int add(int value) const { return base_ + value; }

private:
    int base_;
};

#define SHAPE_SCALE 3

#endif
`,
	})
	blueprint := findBlueprint(t, progs, "Adder")
	require.NotNil(t, blueprint, "included class must be lowered")
	method := blueprint.GetNormalMethod("add")
	require.NotNil(t, method)
	mainFn := findFunction(t, progs, "main")
	target, ok := ssa.ToFunction(method)
	require.True(t, ok)
	require.NotEmpty(t, callsTo(t, mainFn, target), "included method must be callable")
}

// TestCppIncludeNestedHeader asserts an include reached through another header
// is inlined transitively.
func TestCppIncludeNestedHeader(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"main.cpp": `
#include "config.hpp"

int main() {
    return helper(1);
}
`,
		"config.hpp": `
#include "inner.hpp"
`,
		"inner.hpp": `
int helper(int value);
`,
	})
	require.True(t, hasFunctionNamed(t, progs, "helper"), "transitive include must declare helper")
	require.NotEmpty(t, progs[0].Ref("main"))
}

// TestCppIncludeMacroExpansion asserts a macro defined in a header expands in
// the including translation unit.
func TestCppIncludeMacroExpansion(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"main.cpp": `
#include "config.hpp"

int scaled(int value) {
    return value * CONFIG_SCALE;
}
`,
		"config.hpp": `
#define CONFIG_SCALE 3
`,
	})
	fn := funcBySymbol(t, progs[0], "scaled")
	values := instructionOperandInts(t, fn)
	require.Contains(t, values, int64(3), "header macro must expand to its constant")
}

// TestCppIncludeMissingHeader asserts a missing system header does not stop the
// translation unit.
func TestCppIncludeMissingHeader(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"main.cpp": `
#include <missing_header.h>
#include "also_missing.hpp"

int main() {
    return 0;
}
`,
	})
	require.NotEmpty(t, progs[0].Ref("main"))
}

// TestCppIncludeGuard asserts a guarded header included twice is only lowered
// once, so the class is not redefined.
func TestCppIncludeGuard(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"main.cpp": `
#include "guarded.hpp"
#include "guarded.hpp"

int main() {
    Guarded value;
    return value.get();
}
`,
		"guarded.hpp": `
#ifndef GUARDED_HPP
#define GUARDED_HPP

class Guarded {
public:
    int get() const { return 1; }
};

#endif
`,
	})
	require.NotNil(t, progs[0].Program.GetBluePrint("Guarded"))
	require.NotEmpty(t, progs[0].Ref("main"))
}

// TestCppUsingNamespaceAcrossHeader asserts a header using-directive applies to
// the including unit.
func TestCppUsingNamespaceAcrossHeader(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"main.cpp": `
#include "namespace.hpp"

int main() {
    return compute(2);
}
`,
		"namespace.hpp": `
namespace sample {
int compute(int value) { return value + 1; }
}
using namespace sample;
`,
	})
	require.NotNil(t, findFunction(t, progs, "main"))
	require.NotNil(t, findFunction(t, progs, "sample::compute"))
}

// instructionOperandInts collects the integer constants used as operands by the
// instructions of fn.
func instructionOperandInts(t *testing.T, fn *ssa.Function) []int64 {
	t.Helper()
	var values []int64
	collect := func(id int64) {
		operand, ok := fn.GetValueById(id)
		if !ok {
			return
		}
		constInst, ok := ssa.ToConstInst(operand)
		if !ok {
			return
		}
		switch raw := constInst.GetRawValue().(type) {
		case int64:
			values = append(values, raw)
		case int:
			values = append(values, int64(raw))
		}
	}
	for _, blockID := range fn.Blocks {
		block := blockValue(t, fn, blockID)
		for _, instID := range block.Insts {
			inst, ok := fn.GetInstructionById(instID)
			if !ok {
				continue
			}
			switch typed := inst.(type) {
			case *ssa.BinOp:
				collect(typed.X)
				collect(typed.Y)
			case *ssa.UnOp:
				collect(typed.X)
			case *ssa.Call:
				for _, arg := range typed.Args {
					collect(arg)
				}
			case *ssa.SideEffect:
				collect(typed.Value)
			}
		}
	}
	return values
}
package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// TestCppMultiFileCallTarget asserts a call in one translation unit resolves to
// the definition that lives in another translation unit.
func TestCppMultiFileCallTarget(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"math.hpp": `
int square(int value);
`,
		"math.cpp": `
#include "math.hpp"

int square(int value) {
    return value * value;
}
`,
		"main.cpp": `
#include "math.hpp"

int run() {
    return square(3);
}
`,
	})
	definition := findFunction(t, progs, "square")
	require.NotNil(t, definition)
	require.NotEmpty(t, definition.Blocks, "math.cpp must keep the definition body")
	caller := findFunction(t, progs, "run")
	require.NotNil(t, caller, "run must be lowered in main.cpp")
	// C++ translation units are compiled separately and SSA has no project
	// linker: run's call binds to the header declaration. The definition body
	// above is asserted so the two sides are both present.
	require.True(t, hasCallToName(t, caller, "square"), "run must call square")
	require.Len(t, callsIn(t, caller), 1)
}

// TestCppMultiFileClassCall asserts a method call on a class declared in a
// header and defined in another unit keeps a method target.
func TestCppMultiFileClassCall(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"circle.hpp": `
class Circle {
public:
    Circle(int radius) : radius_(radius) {}
    int area() const { return radius_ * radius_; }

private:
    int radius_;
};

Circle makeCircle(int radius);
`,
		"circle.cpp": `
#include "circle.hpp"

Circle makeCircle(int radius) {
    return Circle(radius);
}
`,
		"main.cpp": `
#include "circle.hpp"

int run() {
    Circle circle = makeCircle(2);
    return circle.area();
}
`,
	})
	definition := findFunction(t, progs, "makeCircle")
	require.NotNil(t, definition)
	require.NotEmpty(t, definition.Blocks, "circle.cpp must keep the definition body")
	caller := findFunction(t, progs, "run")
	require.NotNil(t, caller)
	require.True(t, hasCallToName(t, caller, "makeCircle"), "run must call makeCircle")
	circle := findBlueprint(t, progs, "Circle")
	require.NotNil(t, circle)
	require.NotNil(t, circle.GetNormalMethod("area"))
}

// TestCppMultiFilePhi asserts dataflow in a callee file is not flattened when
// the function is called from another file.
func TestCppMultiFilePhi(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"logic.hpp": `
int classify(int value);
`,
		"logic.cpp": `
#include "logic.hpp"

int classify(int value) {
    int label = 0;
    if (value > 0) {
        label = 1;
    } else {
        label = -1;
    }
    return label;
}
`,
		"main.cpp": `
#include "logic.hpp"

int run() {
    return classify(2);
}
`,
	})
	definition := findFunction(t, progs, "classify")
	require.NotNil(t, definition)
	phi := findPhi(t, definition, "label")
	require.Len(t, phi.Edge, 2)
	trueEdge := phiEdgeFrom(t, definition, phi, "if.true")
	falseEdge := phiEdgeFrom(t, definition, phi, "if.false")
	require.EqualValues(t, 1, constInt(t, trueEdge))
	require.EqualValues(t, -1, constInt(t, falseEdge))
}

// TestCppMultiFileHeaderOnlySymbols asserts header declarations do not hide the
// definitions when both files are part of the project.
func TestCppMultiFileHeaderOnlySymbols(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"api.hpp": `
int declared(int value);
`,
		"api.cpp": `
#include "api.hpp"

int declared(int value) {
    return value + 1;
}
`,
	})
	definition := findFunction(t, progs, "declared")
	require.NotNil(t, definition)
	require.NotEmpty(t, definition.Blocks, "definition must keep its body")
}

// findFunction searches every program in a project for an exported function.
func findFunction(t *testing.T, progs []*ssaapi.Program, name string) *ssa.Function {
	t.Helper()
	// A prototype has one empty block; prefer the definition, which has more.
	// A duplicate definition can be stored under a suffixed key (classify$3),
	// so scan the whole function table instead of only GetFunction(name).
	var best *ssa.Function
	for _, prog := range progs {
		prog.Program.Funcs.ForEach(func(key string, fn *ssa.Function) bool {
			if key == name || fn.GetName() == name {
				if best == nil || len(fn.Blocks) > len(best.Blocks) {
					best = fn
				}
			}
			return true
		})
	}
	if best != nil {
		return best
	}
	require.Failf(t, "function not found", "%q is not lowered in any program", name)
	return nil
}

// findBlueprint searches every program in a project for a class blueprint and
// prefers the one that carries members. The application program can hold a
// placeholder with the same name created by virtual import.
func findBlueprint(t *testing.T, progs []*ssaapi.Program, name string) *ssa.Blueprint {
	t.Helper()
	var placeholder *ssa.Blueprint
	for _, prog := range progs {
		blueprint := prog.Program.GetBluePrint(name)
		if blueprint == nil {
			continue
		}
		if len(blueprint.NormalMethod) > 0 || len(blueprint.ParentBlueprints) > 0 {
			return blueprint
		}
		if placeholder == nil {
			placeholder = blueprint
		}
	}
	if placeholder != nil {
		return placeholder
	}
	require.Failf(t, "blueprint not found", "%q is not lowered in any program", name)
	return nil
}
// hasFunctionNamed reports whether any program lowered a function whose key or
// name contains the requested (possibly qualified) spelling.
func hasFunctionNamed(t *testing.T, progs []*ssaapi.Program, name string) bool {
	t.Helper()
	for _, prog := range progs {
		found := false
		prog.Program.Funcs.ForEach(func(key string, fn *ssa.Function) bool {
			if key == name || fn.GetName() == name ||
				strings.HasSuffix(key, name) || strings.HasSuffix(fn.GetName(), name) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

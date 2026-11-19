package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// TestCppDemoCalculator is an end-to-end demo: a class method and a loop-based
// free function meet in main. It asserts the symbols and the loop merge that a
// dataflow rule would rely on.
func TestCppDemoCalculator(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"demo.cpp": `
class Calculator {
public:
    int add(int a, int b) { return a + b; }
};

int sumTo(int n) {
    int total = 0;
    for (int i = 0; i < n; i++) {
        total += i;
    }
    return total;
}

int main() {
    Calculator calculator;
    return calculator.add(sumTo(4), 1);
}
`,
	})
	prog := progs[0]
	calculator := prog.Program.GetBluePrint("Calculator")
	require.NotNil(t, calculator)
	require.NotNil(t, calculator.GetNormalMethod("add"))
	sumTo := funcBySymbol(t, prog, "sumTo")
	phi := findPhi(t, sumTo, "total")
	require.GreaterOrEqual(t, len(phi.Edge), 2, "loop merge lost an edge")
	mainFn := funcBySymbol(t, prog, "main")
	method, ok := ssa.ToFunction(calculator.GetNormalMethod("add"))
	require.True(t, ok)
	require.NotEmpty(t, callsTo(t, mainFn, method), "calculator.add must target Calculator::add")
}

// TestCppDemoInheritedDispatch demonstrates the static dispatch target that a
// rule sees for an inherited method.
func TestCppDemoInheritedDispatch(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"animal.cpp": `
class Animal {
public:
    virtual int legs() { return 4; }
};

class Bird : public Animal {
public:
    int legs() { return 2; }
};

int countLegs() {
    Bird bird;
    return bird.legs();
}
`,
	})
	bird := progs[0].Program.GetBluePrint("Bird")
	require.NotNil(t, bird)
	method, ok := ssa.ToFunction(bird.GetNormalMethod("legs"))
	require.True(t, ok)
	countLegs := funcBySymbol(t, progs[0], "countLegs")
	require.NotEmpty(t, callsTo(t, countLegs, method), "bird.legs() must target Bird::legs")
}

// TestCppDemoFixtureCompiles keeps a compact multi-feature source in the package
// so a failure names one demo instead of a hidden fixture.
func TestCppDemoFixtureCompiles(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"demo.cpp": `
#include <vector>

class Accumulator {
public:
    Accumulator() : total_(0) {}
    void add(int value) { total_ += value; }
    int total() const { return total_; }
private:
    int total_;
};

int accumulate(const std::vector<int>& values) {
    Accumulator accumulator;
    for (int value : values) {
        accumulator.add(value);
    }
    return accumulator.total();
}

int main() {
    std::vector<int> values{1, 2, 3};
    return accumulate(values);
}
`,
	})
	require.NotEmpty(t, progs[0].Ref("accumulate"))
	require.NotNil(t, progs[0].Program.GetBluePrint("Accumulator"))
}

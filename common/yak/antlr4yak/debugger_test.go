package antlr4yak

import (
	"context"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"testing"
)

func RunTestDebugger(code string, debuggerInit, debuggerCallBack func(g *yakvm.Debugger)) {
	engine := New()
	engine.SetDebugMode(true)
	engine.SetDebugInit(debuggerInit)
	engine.SetDebugCallback(debuggerCallBack)
	engine.Eval(context.Background(), code)
}

func TestDebugger_1(t *testing.T) {
	code := `a = 1
dump(a)`
	init := func(g *yakvm.Debugger) {
		g.SetNormalBreakPoint(2)
	}
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		scope := g.VM().CurrentFM().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() != 1 {
			t.Fatal("a != 1 in line 2")
		}
	}

	RunTestDebugger(code, init, callback)
}

func TestDebugger_ConditonalBreakPoint(t *testing.T) {
	code := `a = 1
for range 10 {
	a++
}`
	init := func(g *yakvm.Debugger) {
		err := g.SetCondtionalBreakPoint(3, "a > 5")
		if err != nil {
			t.Fatal(err)
		}
	}

	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		scope := g.VM().CurrentFM().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() <= 5 {
			t.Fatal("conditional breakpoint error")
		}
	}

	RunTestDebugger(code, init, callback)
}

func TestDebugger_StepNext(t *testing.T) {
	code := `a = 1
for range 10 {
	a++
}`
	init := func(g *yakvm.Debugger) {
		err := g.SetNormalBreakPoint(3)
		if err != nil {
			t.Fatal(err)
		}
	}

	next := 0
	callback := func(g *yakvm.Debugger) {
		if next > 2 || g.Finished() {
			return
		}

		checkA := func(wanted int) {
			scope := g.VM().CurrentFM().CurrentScope()
			v, ok := scope.GetValueByName("a")
			if !ok {
				t.Fatal("a not found")
			}
			if v.Int() != wanted {
				t.Fatalf("a(%d) != %d in line %d", v.Int(), wanted, g.CurrentLine())
			}
		}
		checkLine := func(lineIndex int) {
			if g.CurrentLine() != lineIndex {
				t.Fatalf("line %d not reached", lineIndex)
			}
		}

		if next == 0 {
			checkLine(3)
			checkA(1)
			g.StepNext()
			next++
		} else if next == 1 {
			checkLine(2)
			checkA(2)
			g.StepNext()
			next++
		} else if next == 2 {
			checkLine(3)
			checkA(2)
			next++
		}
	}

	RunTestDebugger(code, init, callback)
}

func TestDebugger_BreakPoint_In_Function(t *testing.T) {
	code := `func test() {
a = 1
dump(a)
}

test()`
	init := func(g *yakvm.Debugger) {
		err := g.SetNormalBreakPoint(3)
		if err != nil {
			t.Fatal(err)
		}
	}

	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		scope := g.VM().CurrentFM().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() != 1 {
			t.Fatal("a != 1 in line 3")
		}
	}

	RunTestDebugger(code, init, callback)
}

func TestDebugger_StepIn(t *testing.T) {
	code := `func test() {
a = 1
dump(a)
}
test()
b = 2
c = 3`
	init := func(g *yakvm.Debugger) {
		err := g.SetNormalBreakPoint(5)
		if err != nil {
			t.Fatal(err)
		}
	}
	stepIn := false
	n := 0
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		if !stepIn {
			g.StepIn()
			stepIn = true
		} else if n == 0 {
			g.StepNext()
			n++
		} else if n == 1 {
			g.StepNext()
			n++
		} else if n == 2 {
			scope := g.VM().CurrentFM().CurrentScope()
			v, ok := scope.GetValueByName("a")
			if !ok {
				t.Fatal("a not found")
			}
			if v.Int() != 1 {
				t.Fatal("a != 1 in line 3")
			}
		}
	}

	RunTestDebugger(code, init, callback)
}

func TestDebugger_StepOut(t *testing.T) {
	code := `a = 0
func test() {
a = 1
}
test()
b = 2
c = 3`
	init := func(g *yakvm.Debugger) {
		err := g.SetNormalBreakPoint(5)
		if err != nil {
			t.Fatal(err)
		}
	}
	stepIn, stepOut := false, false
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		if !stepIn {
			g.StepIn()
			stepIn = true
		} else if !stepOut {
			g.StepOut()
			stepOut = true
		} else {
			scope := g.VM().CurrentFM().CurrentScope()
			v, ok := scope.GetValueByName("a")
			if !ok {
				t.Fatal("a not found")
			}
			if v.Int() != 1 {
				t.Fatal("a != 1 after step out")
			}
		}
	}

	RunTestDebugger(code, init, callback)
}

func TestDebugger_Watch(t *testing.T) {
	code := `a = 1
a = 2
a = 3`
	init := func(g *yakvm.Debugger) {
		err := g.AddObserveBreakPoint("a")
		if err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		n++
		scope := g.VM().CurrentFM().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() != n {
			t.Fatalf("a != %d", n)
		}
	}

	RunTestDebugger(code, init, callback)
}

package antlr4yak

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func RunTestDebugger(code string, debuggerInit, debuggerCallBack func(g *yakvm.Debugger)) {
	engine := New()
	// engine
	Import("test_debugger_sleep", func(i int) {
		time.Sleep(time.Duration(i) * time.Second)
	})
	Import("println", func(i ...interface{}) {
		fmt.Println(i...)
	})

	engine.ImportLibs(buildinLib)
	engine.SetDebugMode(true)
	engine.SetDebugInit(debuggerInit)
	engine.SetDebugCallback(debuggerCallBack)
	engine.SetSourceFilePath("/xxx/test.yak")
	engine.Eval(context.Background(), code)
}

func TestDebugger_1(t *testing.T) {
	code := `a = 1
dump(a)`
	in := false
	init := func(g *yakvm.Debugger) {
		g.SetNormalBreakPoint(2)
	}
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		scope := g.Frame().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() != 1 {
			t.Fatal("a != 1 in line 2")
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_Async(t *testing.T) {
	code := `go fn {
a = 1
print(2)
}
test_debugger_sleep(1)`
	in := false
	init := func(g *yakvm.Debugger) {
		g.SetNormalBreakPoint(3)
	}
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		scope := g.Frame().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() != 1 {
			t.Fatal("a != 1 in line 2")
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_ConditonalBreakPoint(t *testing.T) {
	code := `a = 1
for range 10 {
	a++
}`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetBreakPoint(3, "a > 5", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		scope := g.Frame().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() <= 5 {
			t.Fatalf("conditional breakpoint error, a=%d", v.Int())
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_HitConditionBreakPoint(t *testing.T) {
	code := `a = 1
for range 10 {
	a++
}`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetBreakPoint(3, "", "3")
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		scope := g.Frame().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() < 3 {
			t.Fatalf("conditional breakpoint error, a=%d", v.Int())
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_HitConditionBreakPoint2(t *testing.T) {
	code := `a = 1
for range 10 {
	a++
}`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetBreakPoint(3, "a > 3", "3")
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		scope := g.Frame().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() < 6 {
			t.Fatalf("conditional breakpoint error, a=%d", v.Int())
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_HitConditionBreakPoint3(t *testing.T) {
	code := `a = 1
for range 10 {
	a++
}`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetBreakPoint(3, "a > 3", "a > 7")
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		scope := g.Frame().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() < 8 {
			t.Fatalf("conditional breakpoint error, a=%d", v.Int())
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_Continue(t *testing.T) {
	code := `a = 1
for range 10 {
	a++
}`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetNormalBreakPoint(2)
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	n := 0
	callback := func(g *yakvm.Debugger) {
		if n > 4 || g.Finished() {
			return
		}
		in = true

		checkA := func(wanted int) {
			scope := g.Frame().CurrentScope()
			v, ok := scope.GetValueByName("a")
			if !ok {
				t.Fatal("a not found")
			}
			if v.Int() != wanted {
				t.Fatalf("%d: a(%d) != %d in line %d", v.Int(), n, wanted, g.CurrentLine())
			}
		}
		checkLine := func(lineIndex int) {
			if g.CurrentLine() != lineIndex {
				t.Fatalf("%d: line %d not reached, current line: %d", n, lineIndex, g.CurrentLine())
			}
		}

		if n == 0 {
			checkLine(2)
			checkA(1)
		} else if n == 1 {
			checkLine(2)
			checkA(2)
		} else if n == 2 {
			checkLine(2)
			checkA(3)
		} else if n == 3 {
			checkLine(2)
			checkA(4)
		} else if n == 4 {
			checkLine(2)
			checkA(5)
		}
		n++
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	} else if n != 5 {
		t.Fatal("callback not called enough")
	}
}

func TestDebugger_StepNext(t *testing.T) {
	code := `a = 1
for range 10 {
	a++
}`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetNormalBreakPoint(3)
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	n := 0
	callback := func(g *yakvm.Debugger) {
		if n > 4 || g.Finished() {
			return
		}
		in = true

		checkA := func(wanted int) {
			scope := g.Frame().CurrentScope()
			v, ok := scope.GetValueByName("a")
			if !ok {
				t.Fatal("a not found")
			}
			if v.Int() != wanted {
				t.Fatalf("%d: a(%d) != %d in line %d", v.Int(), n, wanted, g.CurrentLine())
			}
		}
		checkLine := func(lineIndex int) {
			if g.CurrentLine() != lineIndex {
				t.Fatalf("%d: line %d not reached, current line: %d", n, lineIndex, g.CurrentLine())
			}
		}

		if n == 0 {
			checkLine(3)
			checkA(1)
			g.StepNext()
		} else if n == 1 {
			checkLine(4)
			checkA(2)
			g.StepNext()
		} else if n == 2 {
			checkLine(2)
			checkA(2)
			g.StepNext()
		} else if n == 3 {
			checkLine(3)
			checkA(2)
			g.StepNext()
		} else if n == 4 {
			checkLine(4)
			checkA(3)
		}
		n++
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	} else if n != 5 {
		t.Fatal("callback not called enough")
	}
}

func TestDebugger_StepNext_JmpFunction(t *testing.T) {
	code := `f = func(v) {
	return v+1
}
a = f(1)
a = f(a)
println(a)
`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetNormalBreakPoint(4)
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	n := 0
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true

		checkA := func(wanted int) {
			scope := g.Frame().CurrentScope()
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

		if n == 0 {
			checkLine(4)
			g.StepNext()
		} else if n == 1 {
			checkLine(5)
			checkA(2)
		}
		n++
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	} else if n != 2 {
		t.Fatal("callback not called enough")
	}
}

func TestDebugger_StepNext_If(t *testing.T) {
	code := `a = 1
if a == 2 {
	println(a)
} else if a == 0 {
	println(a)
} else {
	println(a)
}
`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetNormalBreakPoint(2)
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	n := 0
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		checkLine := func(lineIndex int) {
			if g.CurrentLine() != lineIndex {
				t.Fatalf("line %d not reached", lineIndex)
			}
		}

		if n == 0 {
			checkLine(2)
			g.StepNext()
		} else if n == 1 {
			checkLine(4)
			g.StepNext()
		} else if n == 2 {
			checkLine(6)
			g.StepNext()
		} else if n == 3 {
			checkLine(7)
			g.StepNext()
		} else if n == 4 {
			checkLine(8)
		}
		n++
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	} else if n != 5 {
		t.Fatal("callback not called enough")
	}
}

func TestDebugger_BreakPoint_In_Function(t *testing.T) {
	code := `func test() {
a = 1
dump(a)
}

test()`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetNormalBreakPoint(3)
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		scope := g.Frame().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() != 1 {
			t.Fatal("a != 1 in line 3")
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
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
		_, err := g.SetNormalBreakPoint(5)
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false
	stepIn := false
	n := 0
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
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
			scope := g.Frame().CurrentScope()
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
	if !in {
		t.Fatal("callback not called")
	}
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
		_, err := g.SetNormalBreakPoint(5)
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false
	n := 0
	stepIn, stepOut := false, false
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		n++
		if !stepIn {
			g.StepIn()
			stepIn = true
		} else if !stepOut {
			g.StepOut()
			stepOut = true
		} else {
			scope := g.Frame().CurrentScope()
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
	if !in {
		t.Fatal("callback not called")
	} else if n < 3 {
		t.Fatal("callback not called enough")
	}
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
	in := false
	n := 0
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		n++
		scope := g.Frame().CurrentScope()
		v, ok := scope.GetValueByName("a")
		if !ok {
			t.Fatal("a not found")
		}
		if v.Int() != n {
			t.Fatalf("a != %d", n)
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_StackTrace(t *testing.T) {
	code := `go fn {
	for {
		x = 1	
		test_debugger_sleep(3)
	}
}

test_debugger_sleep(1)

c = func(v) {
	x = v
	d(v)
}

d = func(v) {
x = v
}

a = 1
b = 2
c(a+b)
`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetNormalBreakPoint(16)
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		sts := g.GetStackTraces()
		if len(sts) < 2 {
			t.Fatal("goroutine 1 stack trace not found")
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_Pause(t *testing.T) {
	code := `test_debugger_sleep(1)
a = 1
b = a+1
`
	init := func(g *yakvm.Debugger) {
		g.Pause()
	}
	in := false
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		index := g.CurrentCodeIndex()
		if index != 0 {
			t.Fatal("index != 0")
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_MultiFileDebug(t *testing.T) {
	file, err := os.CreateTemp("", "test*.yak")
	if err != nil {
		panic(err)
	}
	includeCode := `abc = func(){
	a = 1
	println(a+1)
}
`

	file.WriteString(includeCode)
	defer os.Remove(file.Name())

	code := fmt.Sprintf(`include "%s"

abc()
println("finish")`, file.Name())

	init := func(g *yakvm.Debugger) {
		g.SetNormalBreakPoint(3)
	}
	in := false
	stepIn, addObs := false, false
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
		if !stepIn {
			if g.CurrentLine() != 3 {
				t.Fatal("line != 3")
			}
			stepIn = true
			g.StepIn()
		} else if !addObs {
			if g.CurrentLine() != 1 {
				t.Fatal("line != 1")
			}
			addObs = true
			g.AddObserveBreakPoint("a")
		} else {
			scope := g.Frame().CurrentScope()
			v, ok := scope.GetValueByName("a")
			if !ok {
				t.Fatal("a not found")
			}
			if v.Int() != 1 {
				t.Fatalf("a != 1")
			}
		}
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_Try(t *testing.T) {
	t.Run("try", func(t *testing.T) {
		code := `try{
			a=1
		} catch {
			println(0)
		}`
		init := func(g *yakvm.Debugger) {
			_, err := g.SetNormalBreakPoint(2)
			if err != nil {
				t.Fatal(err)
			}
		}
		in := false

		n := 0
		callback := func(g *yakvm.Debugger) {
			if n > 2 || g.Finished() {
				return
			}
			in = true

			checkLine := func(lineIndex int) {
				if g.CurrentLine() != lineIndex {
					t.Fatalf("%d: line %d not reached, current line: %d", n, lineIndex, g.CurrentLine())
				}
			}

			if n == 0 {
				checkLine(2)
				g.StepNext()
			} else if n == 1 {
				checkLine(3)
				g.StepNext()
			} else if n == 2 {
				checkLine(5)
			}
			n++
		}

		RunTestDebugger(code, init, callback)
		if !in {
			t.Fatal("callback not called")
		} else if n != 3 {
			t.Fatal("callback not called enough")
		}
	})
	t.Run("try-catch", func(t *testing.T) {
		code := `try{
			panic("111")
		} catch {
			println(0)
		}`
		init := func(g *yakvm.Debugger) {
			_, err := g.SetNormalBreakPoint(2)
			if err != nil {
				t.Fatal(err)
			}
		}
		in := false

		n := 0
		callback := func(g *yakvm.Debugger) {
			if n > 2 || g.Finished() {
				return
			}
			in = true

			checkLine := func(lineIndex int) {
				if g.CurrentLine() != lineIndex {
					t.Fatalf("%d: line %d not reached, current line: %d", n, lineIndex, g.CurrentLine())
				}
			}

			if n == 0 {
				checkLine(2)
				g.StepNext()
			} else if n == 1 {
				checkLine(3)
				g.StepNext()
			} else if n == 2 {
				checkLine(4)
			}
			n++
		}

		RunTestDebugger(code, init, callback)
		if !in {
			t.Fatal("callback not called")
		} else if n != 3 {
			t.Fatal("callback not called enough")
		}
	})

	t.Run("try-catch-finally", func(t *testing.T) {
		code := `try{
			panic("111")
		} catch {
			println(0)
		} finally {
			println(1)
		}`
		init := func(g *yakvm.Debugger) {
			_, err := g.SetNormalBreakPoint(2)
			if err != nil {
				t.Fatal(err)
			}
		}
		in := false

		n := 0
		callback := func(g *yakvm.Debugger) {
			if n > 4 || g.Finished() {
				return
			}
			in = true

			checkLine := func(lineIndex int) {
				if g.CurrentLine() != lineIndex {
					t.Fatalf("%d: line %d not reached, current line: %d", n, lineIndex, g.CurrentLine())
				}
			}

			if n == 0 {
				checkLine(2)
				g.StepNext()
			} else if n == 1 {
				checkLine(3)
				g.StepNext()
			} else if n == 2 {
				checkLine(4)
				g.StepNext()
			} else if n == 3 {
				checkLine(5)
				g.StepNext()
			} else if n == 4 {
				checkLine(6)
				g.StepNext()
			}
			n++
		}

		RunTestDebugger(code, init, callback)
		if !in {
			t.Fatal("callback not called")
		} else if n != 5 {
			t.Fatal("callback not called enough")
		}
	})

}

func TestDebugger_NestedFunction(t *testing.T) {
	code := `func a() {
	b = func() {
		c = func() {
			println(1)
		}
		c()
	}
	b()
}
a()`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetBreakPoint(4, "", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

func TestDebugger_Recursion(t *testing.T) {
	code := `func fib(a) {
	if a <= 1 {
		return a
	}
	return fib(a-1) + fib(a-2)
}

fib(10)
println(1)`
	init := func(g *yakvm.Debugger) {
		_, err := g.SetBreakPoint(9, "", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	in := false

	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		in = true
	}

	RunTestDebugger(code, init, callback)
	if !in {
		t.Fatal("callback not called")
	}
}

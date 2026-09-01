package antlr4yak

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
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

func TestDebugger_CancelBeforeFirstOpcodeFinishesLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := New()
	engine.SetDebugMode(true)
	engine.SetSourceFilePath("/xxx/cancel-before-first-opcode.yak")

	var debugger *yakvm.Debugger
	var finishedCallbacks atomic.Int32
	engine.SetDebugInit(func(g *yakvm.Debugger) {
		debugger = g
		// Debug init runs after the root frame is installed but before Frame.Exec.
		// This deterministically exercises cancellation before ShouldCallback.
		cancel()
	})
	engine.SetDebugCallback(func(g *yakvm.Debugger) {
		if g.Finished() {
			finishedCallbacks.Add(1)
		}
	})

	err := engine.EvalWithoutCache(ctx, "a = 1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if debugger == nil {
		t.Fatal("debugger init callback was not called")
	}

	started := make(chan struct{})
	go func() {
		debugger.StartWGWait()
		close(started)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("debugger start waiter was not released by terminal callback")
	}

	if !debugger.Finished() {
		t.Fatal("debugger was not marked finished")
	}
	if got := finishedCallbacks.Load(); got != 1 {
		t.Fatalf("expected exactly one finished callback, got %d", got)
	}
}

func TestDebugger_EmptySourceFinishesLifecycle(t *testing.T) {
	for name, source := range map[string]string{
		"empty":        "",
		"comment-only": "// no executable opcode",
	} {
		t.Run(name, func(t *testing.T) {
			engine := New()
			engine.SetDebugMode(true)
			engine.SetSourceFilePath("/xxx/" + name + ".yak")
			engine.SetDebugInit(func(*yakvm.Debugger) {})

			var finishedCallbacks atomic.Int32
			engine.SetDebugCallback(func(g *yakvm.Debugger) {
				if g.Finished() {
					finishedCallbacks.Add(1)
				}
			})

			if err := engine.EvalWithoutCache(context.Background(), source); err != nil {
				t.Fatalf("empty debug execution failed: %v", err)
			}
			debugger := engine.GetVM().GetDebugger()
			if debugger == nil {
				t.Fatal("debugger was not initialized")
			}

			started := make(chan struct{})
			go func() {
				debugger.StartWGWait()
				close(started)
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("empty source left debugger start waiter blocked")
			}

			if !debugger.Finished() {
				t.Fatal("empty source debugger was not marked finished")
			}
			if got := finishedCallbacks.Load(); got != 1 {
				t.Fatalf("expected exactly one finished callback, got %d", got)
			}
		})
	}
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

func TestDebugger_StepInVariadicCall(t *testing.T) {
	code := `result = 0
func collect(a, b, c) {
result = a + b + c
}
args = [1, 2, 3]
collect(args...)
assert result == 6`
	requestedStepIn := false
	enteredFunction := false
	init := func(g *yakvm.Debugger) {
		_, err := g.SetNormalBreakPoint(6)
		if err != nil {
			t.Fatal(err)
		}
	}
	callback := func(g *yakvm.Debugger) {
		if g.Finished() {
			return
		}
		if !requestedStepIn {
			requestedStepIn = true
			if err := g.StepIn(); err != nil {
				t.Fatal(err)
			}
			return
		}
		if g.StateName() == "collect" {
			enteredFunction = true
		}
	}

	RunTestDebugger(code, init, callback)
	if !requestedStepIn {
		t.Fatal("variadic call breakpoint was not reached")
	}
	if !enteredFunction {
		t.Fatal("step-in treated an expanded argument as the callable")
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
	for i := 0; i < 1; i++ {
		x = 1	
		test_debugger_sleep(2)
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
waitAllAsyncCallFinish()
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
		// The background goroutine is running rather than paused, so the
		// debugger's visibility contract reports only the stopped root thread.
		// Synchronous c -> d frames must retain that root thread ID instead of
		// drifting to the last allocated async ID.
		trace, ok := sts[g.CurrentThreadID()]
		if !ok {
			t.Fatal("current root thread stack trace not found")
		}
		if len(sts) != 1 {
			t.Fatalf("unexpected visible thread count: %d", len(sts))
		}
		if len(trace.StackTraces) < 3 {
			t.Fatalf("nested c -> d stack trace too short: %d", len(trace.StackTraces))
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

	// Windows 路径含 `\Users` 会被 lexer 当成 `\uXXXX` 转义，include 字面量统一用 /
	code := fmt.Sprintf(`include "%s"

abc()
println("finish")`, filepath.ToSlash(file.Name()))

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

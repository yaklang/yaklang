package antlr4yak

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklib/container"
)

func TestCoreHardeningSemantics(t *testing.T) {
	for _, synchronous := range []bool{false, true} {
		t.Run(fmt.Sprint(synchronous), func(t *testing.T) {
			e := New()
			e.GetVM().GetConfig().SetSynchronousExecution(synchronous)
			e.ImportLibs(map[string]any{"array": [3]int{1, 2, 3}, "minStep": math.MinInt, "maxStep": math.MaxInt,
				"size": func(v any) int { return reflect.ValueOf(v).Len() },
				"drop": func(m map[string]int, key string) { delete(m, key) },
			})
			const source = `
assert [] == [][:]
assert "" == ""[:]
assert [] == [][::-1]
a = [1,2,3]
assert size(a[3:3]) == 0
assert size(a[0:0:-1]) == 0
assert a[::-1] == [3,2,1]
assert a[0::-1] == [1]
assert size(a[:-1:-1]) == 0
assert a[::] == [1,2,3]
assert a[::maxStep] == [1]
assert a[::-maxStep] == [3]
assert a[::minStep] == [3]
b = a[:]
b[0] = 99
assert a[0] == 1
assert array[:] == [1,2,3]
assert array[1:2] == [2]
assert "甲🙂乙"[::-1] == "乙🙂甲"
assert [nil] == [undefined]
assert [nil,1][1] == 1
assert [1,nil][0] == 1
assert 9 - 4 == 5
assert 12 / 3 == 4
assert 3 << 2 == 12
x, y = 1, 2
x, y = y, x
assert x == 2 && y == 1
x -= y
assert x == 1
m = {"a":1,"b":2,"c":3}
visited = []
for k, value = range m {
    visited.Append(k)
    if k == "a" { drop(m, "b") }
}
assert visited == ["a", "c"]
inc = () => { x += 1 }
f = () => { defer inc(); return 3 }
assert f() == 3
assert x == 2
`
			if err := e.SafeEval(context.Background(), source); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCoreBlockedChannelInstructionCancellation(t *testing.T) {
	for _, source := range []string{"ch <- 1", "<-ch", "for v = range ch {}"} {
		t.Run(source, func(t *testing.T) {
			e := New()
			e.ImportLibs(map[string]any{"ch": (chan int)(nil)})
			codes, err := e.Compile(source)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- e.GetVM().ExecYakCode(ctx, source, codes) }()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("cancellation: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("instruction remained blocked")
			}
		})
	}
}

func TestCoreAsyncScriptErrorsAndSynchronousBoundary(t *testing.T) {
	for _, synchronous := range []bool{false, true} {
		for _, source := range []string{"go host()", "f = () => { panic(\"yak boom\") }; go f()", "go hostCallback(() => { return 1 })"} {
			e := New()
			e.GetVM().GetConfig().SetSynchronousExecution(synchronous)
			e.ImportLibs(map[string]any{
				"host": func() { panic("host boom") },
				"hostCallback": func(fn func() int) {
					if fn() != 1 {
						panic("wrong callback result")
					}
					panic("callback boom")
				},
			})
			err := func() (err error) {
				defer func() {
					if p := recover(); p != nil {
						err = fmt.Errorf("%v", p)
					}
				}()
				return e.Eval(context.Background(), source)
			}()
			if synchronous {
				if err == nil || !strings.Contains(err.Error(), "synchronous execution") {
					t.Fatalf("%s: %v", source, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err := e.GetVM().AsyncWaitError(); err == nil || !strings.Contains(err.Error(), "boom") {
					t.Fatalf("%s: %v", source, err)
				}
			}
		}
	}
}

func TestCoreInvalidCompileThenReuse(t *testing.T) {
	e := New()
	for _, source := range []string{"1 = 2", "f() = 2", "go 1+2", "defer x"} {
		codes, err := e.Compile(source)
		if err == nil || len(codes) != 0 {
			t.Fatalf("accepted partial compilation %q: %v", source, err)
		}
		if err := e.Eval(context.Background(), "assert 2+3 == 5"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCoreSetIterationExitClosesProducer(t *testing.T) {
	for _, source := range []string{
		"for v in values { break }",
		"f=()=>{ for v in values { return v } }; f()",
		"for v in values { panic(1) }",
		"for v in values { cancel() }",
	} {
		t.Run(source, func(t *testing.T) {
			values := container.NewSet(1, 2, 3)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e := New()
			e.GetVM().GetConfig().SetSuppressPanicDebugStack(true)
			e.ImportLibs(map[string]any{"values": values, "cancel": cancel})
			err := e.SafeEval(ctx, source)
			if strings.Contains(source, "panic") && err == nil {
				t.Fatal("panic was lost")
			}
			if strings.Contains(source, "cancel") && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation: %v", err)
			}
			done := make(chan struct{})
			go func() { values.Add(4); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("iterator retained set lock after execution exit")
			}
		})
	}
}

func TestCoreBytecodeRoundTrip(t *testing.T) {
	for _, source := range []string{
		"assert 2+3 == 5",
		"f = (a,rest...) => { defer mark(); return a+size(rest) }; assert f(1,2,3) == 3",
		"a = [1,2,3]; assert a[::-1] == [3,2,1]; assert size(a[0:0:-1]) == 0",
		"total=0; for i in 5 { if i==3 { break }; total+=i }; assert total==3",
		"try { panic(1) } catch e { assert e != nil }",
	} {
		t.Run(source, func(t *testing.T) {
			e := New()
			e.ImportLibs(map[string]any{"size": func(v any) int { return reflect.ValueOf(v).Len() }, "mark": func() {}})
			codes, err := e.Compile(source)
			if err != nil {
				t.Fatal(err)
			}
			m := yakvm.NewCodesMarshaller()
			payload, err := m.Marshal(e.rootSymbol, codes)
			if err != nil {
				t.Fatal(err)
			}
			table, decoded, err := m.Unmarshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			e.GetVM().SetSymboltable(table)
			if err := e.GetVM().ExecYakCode(context.Background(), source, decoded); err != nil {
				t.Fatal(err)
			}
		})
	}
}

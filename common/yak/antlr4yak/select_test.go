package antlr4yak

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestSelectExecution(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"receive", `select { case v := <-ch: assert v == 7; default: panic("default") }`},
		{"receive ok", `select { case v, ok := (<-ch): assert v == 7 && ok }`},
		{"discard receive", `select { case <-ch: }`},
		{"assignment", `v = 0; ok = false; select { case v, ok = <-ch: }; assert v == 7 && ok`},
		{"case scope", `v = 9; select { case v := <-ch: assert v == 7 }; assert v == 9`},
		{"default first", `select { default: panic("default"); case v := <-ch: assert v == 7 }`},
		{"send", `select { case out <- 8: }; assert <-out == 8`},
		{"default", `select { case <-empty: panic("receive"); case empty <- 1: panic("send"); default: }`},
		{"default only", `select { default: }; assert 1 == 1`},
		{"nil disabled", `select { case <-disabled: panic("nil"); default: }`},
		{"closed", `select { case v, ok := <-closed: assert v == nil && !ok }`},
		{"break", `x = 0; select { case <-ch: x = 1; break; x = 2 }; assert x == 1`},
		{"nested switch", `select { case <-ch: switch 1 { case 1: fallthrough; default: assert true }; break }`},
		{"return defer", `f = () => { defer mark(); select { case v := <-ch: return v; default: return -1 } }; assert f() == 7`},
		{"loop continue", `n=0; for i in 3 { select { default: n++; continue }; panic("continue") }; assert n == 3`},
		{"loop break", `n=0; for i in 3 { select { default: break }; n++ }; assert n == 3`},
		{"nested select", `select { case <-ch: select { default: break }; break }; assert true`},
		{"try break", `select { default: try { break } catch e { panic(e) } }; assert true`},
		{"identifier", `select = 3; assert select == 3; f=(select)=>select+1; assert f(2)==3; m={"select":()=>7}; assert m.select()==7`},
		{"statement boundary", "select=1\nselect\n{ assert select == 1 }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := New()
			ch := make(chan int, 1)
			ch <- 7
			closed := make(chan int)
			close(closed)
			marked := false
			e.ImportLibs(map[string]any{"ch": ch, "out": make(chan int, 1), "empty": make(chan int), "disabled": (chan int)(nil), "closed": closed, "mark": func() { marked = true }})
			if err := e.SafeEval(context.Background(), tc.source); err != nil {
				t.Fatal(err)
			}
			if tc.name == "return defer" && !marked {
				t.Fatal("defer did not run in containing function")
			}
		})
	}
}

func TestSelectOneCommunication(t *testing.T) {
	for _, send := range []bool{false, true} {
		for i := 0; i < 100; i++ {
			a, b := make(chan int, 1), make(chan int, 1)
			source := `select { case <-a: case <-b: }`
			if send {
				source = `select { case a <- 1: case b <- 2: }`
			} else {
				a <- 1
				b <- 2
			}
			e := New()
			e.ImportLibs(map[string]any{"a": a, "b": b})
			if err := e.SafeEval(context.Background(), source); err != nil {
				t.Fatal(err)
			}
			if len(a)+len(b) != 1 {
				t.Fatal("select performed more than one communication")
			}
		}
	}
}

func TestSelectOperandOrderAndDeferredLeft(t *testing.T) {
	ch := make(chan int, 1)
	ch <- 7
	var order []string
	channels := func(name string) chan int { order = append(order, name); return ch }
	value := func() int { order = append(order, "send value"); return 9 }
	left := func() int { order = append(order, "left"); return 0 }
	e := New()
	e.ImportLibs(map[string]any{"channel": channels, "value": value, "left": left, "disabled": (chan int)(nil)})
	if err := e.SafeEval(context.Background(), `a=[0]; select { case a[left()] = <-channel("receive"): assert a[0]==7; case disabled <- value(): panic("nil send") }`); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"receive", "send value", "left"}) {
		t.Fatalf("operand order: %v", order)
	}
	order = nil
	if err := e.SafeEval(context.Background(), `a=[0]; select { case a[left()] = <-disabled: panic("nil recv"); case channel("send") <- value(): }`); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"send", "send value"}) {
		t.Fatalf("losing left evaluated: %v", order)
	}
}

func TestSelectPreservesValues(t *testing.T) {
	for _, payload := range []any{int64(42), uint64(1<<63 + 9), float32(1.25), map[string]int{"x": 1}, []int{1, 2}, (*int)(nil), nil} {
		t.Run(fmt.Sprintf("%T", payload), func(t *testing.T) {
			ch := make(chan any, 1)
			e := New()
			e.ImportLibs(map[string]any{"ch": ch, "payload": payload})
			if err := e.SafeEval(context.Background(), `select { case ch <- payload: }`); err != nil {
				t.Fatal(err)
			}
			got := <-ch
			if !reflect.DeepEqual(got, payload) {
				t.Fatalf("send changed %T(%v) into %T(%v)", payload, payload, got, got)
			}
			if m, ok := got.(map[string]int); ok {
				m["x"] = 2
				if payload.(map[string]int)["x"] != 2 {
					t.Fatal("map identity changed")
				}
			}
			if s, ok := got.([]int); ok {
				s[0] = 2
				if payload.([]int)[0] != 2 {
					t.Fatal("slice identity changed")
				}
			}
			// Receive bindings use Yak's existing assignment normalization. Compare
			// exact dynamic types/values with an ordinary receive of the same payload.
			ch <- payload
			if err := e.SafeEval(context.Background(), `baseline = <-ch`); err != nil {
				t.Fatal(err)
			}
			baseline, _ := e.GetVar("baseline")
			ch <- payload
			if err := e.SafeEval(context.Background(), `received = undefined; select { case received = <-ch: }`); err != nil {
				t.Fatal(err)
			}
			if got, _ := e.GetVar("received"); !reflect.DeepEqual(got, baseline) {
				t.Fatalf("receive changed type/value: %T(%v)", got, got)
			}
		})
	}
}

func TestSelectCancellation(t *testing.T) {
	for _, source := range []string{`select {}`, `select { case <-ch: }`, `select { case ch <- 1: }`, `select { case <-ch: case ch <- 1: }`} {
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
				t.Fatal("select remained blocked")
			}
		})
	}
}

func TestSelectInvalidCases(t *testing.T) {
	for _, source := range []string{`select { default: default: }`, `select { case 1: }`, `select { case f(): }`, `select { case x:=1: }`, `select { case x++: }`, `select { case x,y,z:=<-ch: }`, `select { case a.x:=<-ch: }`, `select { case x:=<-ch,1: }`, `select { default: fallthrough }`, `select { default: if true { fallthrough } }`, `select { case (<-ch)+1: }`} {
		t.Run(source, func(t *testing.T) {
			e := New()
			codes, err := e.Compile(source)
			if err == nil || len(codes) != 0 {
				t.Fatalf("accepted invalid select: %v (%d codes)", err, len(codes))
			}
		})
	}
}

func TestSelectChannelValidation(t *testing.T) {
	closed := make(chan int)
	close(closed)
	for _, tc := range []struct {
		source  string
		ch      any
		payload any
		message string
	}{
		{`select {case <-ch:}`, 1, nil, "requires a channel"},
		{`select {case <-ch:}`, (chan<- int)(make(chan int, 1)), nil, "send-only"},
		{`select {case ch <- 1:}`, (<-chan int)(make(chan int, 1)), nil, "receive-only"},
		{`select {case ch <- payload:}`, make(chan int, 1), "bad", "cannot send"},
		{`select {case ch <- payload:}`, make(chan int, 1), nil, "cannot send nil"},
		{`select {case ch <- 1:}`, closed, nil, "closed channel"},
	} {
		t.Run(tc.message, func(t *testing.T) {
			e := New()
			e.GetVM().GetConfig().SetSuppressPanicDebugStack(true)
			e.ImportLibs(map[string]any{"ch": tc.ch, "payload": tc.payload})
			err := e.SafeEval(context.Background(), tc.source)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("wanted %q: %v", tc.message, err)
			}
		})
	}
}

func TestSelectClosedBufferedChannel(t *testing.T) {
	ch := make(chan int, 1)
	ch <- 7
	close(ch)
	e := New()
	e.ImportLibs(map[string]any{"ch": ch})
	if err := e.SafeEval(context.Background(), `select {case v,ok:=<-ch: assert v==7 && ok}; select {case v,ok:=<-ch: assert v==nil && !ok}`); err != nil {
		t.Fatal(err)
	}
}

func TestSelectBlockingWakeup(t *testing.T) {
	for _, send := range []bool{false, true} {
		t.Run(fmt.Sprint(send), func(t *testing.T) {
			ch := make(chan int)
			e := New()
			e.ImportLibs(map[string]any{"ch": ch})
			source := `select {case v:=<-ch: assert v==7}`
			if send {
				source = `select {case ch<-7:}`
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- e.SafeEval(ctx, source) }()
			if send {
				select {
				case v := <-ch:
					if v != 7 {
						t.Fatalf("got %v", v)
					}
				case <-ctx.Done():
					t.Fatal("send did not wake")
				}
			} else {
				select {
				case ch <- 7:
				case <-ctx.Done():
					t.Fatal("receive did not wake")
				}
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelectConcurrentFramesAndSandbox(t *testing.T) {
	ch := make(chan int, 16)
	for i := 0; i < 16; i++ {
		ch <- i
	}
	results := make(chan int, 16)
	e := New()
	e.ImportLibs(map[string]any{"ch": ch, "record": func(v int) { results <- v }})
	source := `select {case v:=<-ch: record(v)}`
	codes, err := e.Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errors := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- e.GetVM().ExecYakCode(ctx, source, codes) }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[int]bool{}
	for i := 0; i < 16; i++ {
		select {
		case v := <-results:
			if seen[v] {
				t.Fatalf("duplicate receive: %d", v)
			}
			seen[v] = true
		case <-ctx.Done():
			t.Fatal("missing receive")
		}
	}
	if len(seen) != 16 || len(ch) != 0 {
		t.Fatal("concurrent select lost communication")
	}
	if err := e.SafeEval(context.Background(), `f=()=>{select {default: return 7}}`); err != nil {
		t.Fatal(err)
	}
	f, _ := e.GetVar("f")
	other := New()
	other.SetSandboxMode(true)
	other.ImportLibs(map[string]any{"f": f})
	if err := other.SafeEval(context.Background(), `assert f()==7`); err != nil {
		t.Fatal(err)
	}
	other.GetVM().GetConfig().SetSynchronousExecution(true)
	if err := other.SafeEval(context.Background(), `select {default: assert true}`); err != nil {
		t.Fatal(err)
	}
}

func TestSelectFormattingComments(t *testing.T) {
	source := `select {
// before case
case v := <-ch:
    // inside case
    assert v == 7 // trailing
default:
    /* block comment */
}`
	e := New()
	formatted, err := e.FormattedAndSyntaxChecking(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, comment := range []string{"// before case", "// inside case", "// trailing", "/* block comment */"} {
		if !strings.Contains(formatted, comment) {
			t.Fatalf("lost %q:\n%s", comment, formatted)
		}
	}
	again, err := e.FormattedAndSyntaxChecking(formatted)
	if err != nil || again != formatted {
		t.Fatalf("comment format not stable: %v\n%s\n%s", err, formatted, again)
	}
	ch := make(chan int, 1)
	ch <- 7
	e.ImportLibs(map[string]any{"ch": ch})
	if err := e.SafeEval(context.Background(), formatted); err != nil {
		t.Fatal(err)
	}
}

func TestSelectSLLAndLL(t *testing.T) {
	for _, mode := range []string{"1", "0"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("YAK_ANTLR_SLL_FIRST", mode)
			e := New()
			if err := e.SafeEval(context.Background(), `select=1; m={"select":()=>2}; assert m.select()==2; select {default: assert select==1}`); err != nil {
				t.Fatal(err)
			}
			if _, err := e.Compile(`other {default:}`); err == nil {
				t.Fatal("accepted another identifier as select")
			}
		})
	}
}

func TestSelectSLLFastPath(t *testing.T) {
	for _, source := range []string{`select {default:}`, `select {case v,ok:=<-ch: use(v,ok); case out<-1:}`, `select=1`, `object.select()`, "select\n{ use(select) }"} {
		_, bailed, err := sllBailParse(source + "\n")
		if bailed || err != nil {
			t.Fatalf("SLL fallback for %q: bail=%v err=%v", source, bailed, err)
		}
	}
}

func TestSelectEmptyBytecodeCancellation(t *testing.T) {
	e := New()
	data, err := e.Marshal(`select {}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	other := New()
	err = other.SafeExecYakc(ctx, data, nil, `select {}`)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("empty bytecode select did not cancel: %v", err)
	}
}

func TestSelectBytecodeAndFormatting(t *testing.T) {
	source := `select { default: panic("default"); case v, ok := <-ch: assert v == 7 && ok; case out <- 8: panic("send") }`
	e := New()
	e.SetStrictMode(true)
	e.ImportLibs(map[string]any{"ch": make(chan int), "out": (chan int)(nil)})
	formatted, err := e.FormattedAndSyntaxChecking(source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(formatted, "select {") || !strings.Contains(formatted, "v, ok := <-ch") {
		t.Fatalf("format lost select: %s", formatted)
	}
	again, err := e.FormattedAndSyntaxChecking(formatted)
	if err != nil || again != formatted {
		t.Fatalf("format not stable: %v\n%s\n%s", err, formatted, again)
	}
	data, err := e.Marshal(formatted, nil)
	if err != nil {
		t.Fatal(err)
	}
	other := New()
	ch := make(chan int, 1)
	ch <- 7
	other.ImportLibs(map[string]any{"ch": ch, "out": (chan int)(nil)})
	if err := other.SafeExecYakc(context.Background(), data, nil, formatted); err != nil {
		t.Fatal(err)
	}
	// Only existing wire opcodes and serializable operands are required.
	_, codes, err := other.UnMarshal(data, nil, formatted)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range codes {
		if c.Opcode == yakvm.OpPushId && c.Op1.String() == yakvm.SelectBuiltinName {
			found = true
		}
	}
	if !found {
		t.Fatal("missing serialized internal select reference")
	}
}

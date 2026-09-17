package yakvm

import (
	"context"
	"errors"
	"fmt"
	mapset "github.com/deckarep/golang-set/v2"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func corePanic(fn func()) (result any) {
	defer func() { result = recover() }()
	fn()
	return
}

func TestCoreNativeAsyncPanicSubprocess(t *testing.T) {
	if os.Getenv("YAKVM_ASYNC_PANIC_CHILD") == "1" {
		vm := New()
		frame := NewFrame(vm)
		for _, fn := range []any{func() { panic("host boom") }, (func())(nil)} {
			NewAutoValue(fn).NativeAsyncCall(frame, false)
		}
		if err := frame.WaitAsync(); err == nil || !strings.Contains(err.Error(), "host boom") {
			t.Fatalf("missing execution error: %v", err)
		}
		if err := vm.AsyncWaitError(); err == nil {
			t.Fatal("missing VM error")
		}
		if err := vm.AsyncWaitError(); err != nil {
			t.Fatalf("errors were not drained: %v", err)
		}
		if frame.coroutine.lastPanic != nil {
			t.Fatal("worker modified parent panic state")
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCoreNativeAsyncPanicSubprocess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "YAKVM_ASYNC_PANIC_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("async worker killed or blocked host: %v\n%s", err, out)
	}
}

func TestCoreAsyncErrorsBoundedAndIsolated(t *testing.T) {
	vm := New()
	a, b := NewFrame(vm), NewFrame(vm)
	for i := 0; i < maxAsyncErrors+10; i++ {
		NewAutoValue(func() { panic("worker") }).NativeAsyncCall(a, false)
	}
	NewAutoValue(func() {}).NativeAsyncCall(b, false)
	if err := b.WaitAsync(); err != nil {
		t.Fatalf("cross-execution error: %v", err)
	}
	if err := a.WaitAsync(); err == nil || !strings.Contains(err.Error(), "10 additional") {
		t.Fatalf("unexpected bounded errors: %v", err)
	}
	vm.AsyncWait()
	if len(a.asyncExecution.errors.errors) != maxAsyncErrors {
		t.Fatal("unbounded error collector")
	}
}

func TestCoreAsyncValidationDoesNotRegisterWorker(t *testing.T) {
	vm := New()
	f := NewFrame(vm)
	if p := corePanic(func() { NewAutoValue(func(int) {}).NativeAsyncCall(f, false, NewAutoValue("wrong")) }); p == nil {
		t.Fatal("expected conversion failure")
	}
	done := make(chan struct{})
	go func() { vm.AsyncWait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("validation leaked wait count")
	}
}

func TestCoreCrossVMAsyncErrorsBelongToCaller(t *testing.T) {
	definition, caller := New(), New()
	caller.SetSandboxMode(true)
	definingFrame, parent := NewFrame(definition), NewFrame(caller)
	f := NewFunction([]*Code{{Opcode: OpPush, Op1: NewAutoValue(func() { panic("nested host boom") })}, {Opcode: OpAsyncCall}}, definition.rootScope.symtbl)
	f.defineFrame, f.scope = definingFrame, definition.rootScope
	if err := caller.ExecAsyncYakFunction(context.Background(), parent, f, nil); err != nil {
		t.Fatal(err)
	}
	if err := parent.WaitAsync(); err == nil || !strings.Contains(err.Error(), "nested host boom") {
		t.Fatalf("caller lost nested worker failure: %v", err)
	}
	if err := definingFrame.WaitAsync(); err != nil {
		t.Fatalf("error leaked to closure definition execution: %v", err)
	}
	caller.AsyncWait()
	definition.AsyncWait()
}

func TestCoreScopeSnapshotConcurrent(t *testing.T) {
	table := NewSymbolTable()
	id, _ := table.NewSymbolWithReturn("value")
	outer := NewScope(table)
	outer.NewValueByID(id, NewAutoValue(-1))
	innerTable := table.CreateSubSymbolTable()
	innerID, _ := innerTable.NewSymbolWithReturn("value")
	s := outer.CreateSubScope(innerTable)
	s.NewValueByID(innerID, NewAutoValue(0))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for n := 0; n < 2000; n++ {
			s.NewValueByID(innerID, NewAutoValue(n))
		}
	}()
	for n := 0; n < 2000; n++ {
		if s.Len() != 1 {
			t.Fatal("wrong binding count")
		}
		if s.GetAllNameAndValueInScopes()["value"] == nil {
			t.Fatal("missing local")
		}
		if got := s.GetAllNameAndValueInAllScopes()["value"]; got == nil || got.Int() < 0 {
			t.Fatal("outer scope shadowed inner")
		}
	}
	wg.Wait()
}

func TestCoreVariadicBinding(t *testing.T) {
	f := NewFunction(nil, NewSymbolTable())
	f.SetParamSymbols([]int{1, 2})
	f.SetIsVariableParameter(true)
	for _, count := range []int{0, 1, 2, 64} {
		for _, check := range []bool{false, true} {
			t.Run(fmt.Sprintf("n%d/check%v", count, check), func(t *testing.T) {
				vs := make([]*Value, count)
				for i := range vs {
					vs[i] = NewAutoValue(i)
				}
				var bound map[int]*Value
				p := corePanic(func() { bound = YakVMValuesToFunctionMap(f, vs, check) })
				if count == 0 && check {
					if p == nil {
						t.Fatal("missing arity error")
					}
					return
				}
				if p != nil {
					t.Fatalf("binding panicked: %v", p)
				}
				if count == 0 && !IsUndefined(bound[1]) {
					t.Fatal("missing fixed argument must be undefined")
				}
				if got := len(bound[2].Value.([]interface{})); got != max(0, count-1) {
					t.Fatalf("tail length %d", got)
				}
			})
		}
	}
	named := NewAutoValue(7)
	named.SymbolId = 1
	bound := YakVMValuesToFunctionMap(f, []*Value{named, NewAutoValue(8), NewAutoValue(9)}, true)
	if !reflect.DeepEqual(bound[2].Value, []interface{}{8, 9}) {
		t.Fatalf("named parameter consumed tail: %v", bound)
	}
}

func TestCoreNativeVariadicDiagnostic(t *testing.T) {
	f := NewFrame(New())
	for _, tc := range []struct {
		fn       any
		args     []*Value
		position string
	}{
		{func(...int) {}, []*Value{NewAutoValue(1), NewAutoValue(map[string]int{})}, "argument 2"},
		{func(int, ...int) {}, []*Value{NewAutoValue(1), NewAutoValue(2), NewAutoValue(3), NewAutoValue("bad")}, "argument 4"},
	} {
		p := fmt.Sprint(corePanic(func() { NewAutoValue(tc.fn).NativeCall(f, false, tc.args...) }))
		if !strings.Contains(p, tc.position) || !strings.Contains(p, "auto convert") || strings.Contains(p, "out of range") {
			t.Fatalf("lost conversion diagnostic: %s", p)
		}
	}
}

type coreNamedString string
type coreNamedBytes []byte
type coreStringer struct{}

func (coreStringer) String() string { return "value" }

func TestCoreReflectConversions(t *testing.T) {
	stringer := reflect.TypeOf((*fmt.Stringer)(nil)).Elem()
	for _, tc := range []struct {
		source any
		target reflect.Type
		ok     bool
	}{
		{1, stringer, false}, {coreStringer{}, stringer, true},
		{[]int{1, 2}, reflect.TypeOf([2]int{}), true},
		{[]int{1}, reflect.TypeOf([2]int{}), false},
		{[2]int{1, 2}, reflect.TypeOf([]int64{}), true},
		{coreNamedString("abc"), reflect.TypeOf(coreNamedBytes{}), true},
		{coreNamedBytes("abc"), reflect.TypeOf(coreNamedString("")), true},
		{(*coreStringer)(nil), reflect.TypeOf(coreStringer{}), false},
		{make(chan int), reflect.TypeOf(make(chan string)), false},
	} {
		v := reflect.ValueOf(tc.source)
		err := (*Frame)(nil).AutoConvertReflectValueByType(&v, tc.target)
		if (err == nil) != tc.ok {
			t.Fatalf("%T -> %v: %v", tc.source, tc.target, err)
		}
		if err == nil && !v.Type().AssignableTo(tc.target) {
			t.Fatalf("conversion returned incompatible %v", v.Type())
		}
	}
}

func TestCoreNilTypeInference(t *testing.T) {
	for _, values := range [][]*Value{{NewAutoValue(nil)}, {undefined}, {nil}, {NewAutoValue(nil), NewAutoValue(1)}, {NewAutoValue(1), NewAutoValue(nil)}} {
		if got := GuessValuesTypeToBasicType(values...); got != literalReflectType_Interface {
			t.Fatalf("nil literal type: %v", got)
		}
	}
}

func TestCoreChannelSendCancellation(t *testing.T) {
	full := make(chan int, 1)
	full <- 7
	for name, ch := range map[string]chan int{"nil": nil, "unbuffered": make(chan int), "full": full} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			frame := NewFrame(New())
			frame.ctx = ctx
			done := make(chan struct{})
			go func() { defer close(done); frame.sendChannel(NewAutoValue(ch), NewAutoValue(1)) }()
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("send ignored cancellation")
			}
		})
	}
	if <-full != 7 {
		t.Fatal("cancelled send replaced buffered value")
	}
	frame := NewFrame(New())
	ch := make(chan *int, 1)
	frame.sendChannel(NewAutoValue(ch), NewAutoValue(nil))
	if <-ch != nil {
		t.Fatal("nil send changed value")
	}
	closed := make(chan int)
	close(closed)
	if p := corePanic(func() { frame.sendChannel(NewAutoValue(closed), NewAutoValue(1)) }); p == nil || !strings.Contains(fmt.Sprint(p), "closed channel") {
		t.Fatalf("closed channel: %v", p)
	}
	if p := corePanic(func() { frame.sendChannel(NewAutoValue((<-chan int)(closed)), NewAutoValue(1)) }); p == nil || !strings.Contains(fmt.Sprint(p), "receive-only") {
		t.Fatalf("direction: %v", p)
	}
}

func TestCoreOperandStackOrderAndRelease(t *testing.T) {
	f := NewFrame(New())
	a, b := NewAutoValue(1), NewAutoValue(2)
	f.push(a)
	f.push(b)
	storage := f.stack.values[:cap(f.stack.values)]
	left, right := f.pop2()
	if left != a || right != b {
		t.Fatal("operand order reversed")
	}
	for _, value := range storage {
		if value != nil {
			t.Fatal("pop retained operand")
		}
	}
	if f.peekN(-1) != nil || f.peekN(0) != nil {
		t.Fatal("out of bounds peek")
	}
}

func TestCoreFuzzTagPartialErrorPushesOnce(t *testing.T) {
	f := NewFrame(New())
	f.push(NewAutoValue("sentinel"))
	f.pushFuzzTagResult([]string{"partial"}, errors.New("injected fuzz tag failure"))
	if f.stack.Len() != 2 || len(f.pop().Value.([]string)) != 0 || f.pop().Value != "sentinel" {
		t.Fatal("failed tag unbalanced operand stack")
	}
}

func TestCoreSetIteratorCloseReleasesProducer(t *testing.T) {
	s := mapset.NewSet[any](1, 2, 3)
	iter := newSetIterator(s)
	iter.Next()
	iter.Close()
	iter.Close()
	done := make(chan struct{})
	go func() { s.Add(4); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("set producer retained read lock after Close")
	}
}

func TestCoreSliceResultCapacity(t *testing.T) {
	f := NewFrame(New())
	f.push(NewAutoValue(make([]int, 100000)))
	f.push(NewAutoValue(1))
	f.push(NewAutoValue(2))
	f.push(NewAutoValue(false))
	f._execCode(&Code{Opcode: OpIterableCall, Unary: 2, Op1: NewAutoValue(0)}, false)
	result := f.pop().Value.([]int)
	if len(result) != 1 || cap(result) != 1 {
		t.Fatalf("tiny slice retained input capacity: %d/%d", len(result), cap(result))
	}
}

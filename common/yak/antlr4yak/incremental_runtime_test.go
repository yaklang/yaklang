package antlr4yak

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestCoreIncrementalRuntimeShapes(t *testing.T) {
	for _, template := range []string{
		`result=%d; for i in 17 { result=result+1 }; assert result==%d+17`,
		`result=%d; if result%%2==0 { result=result+2 } else { result=result+3 }; assert result>=%d+2`,
		`result=%d; f=()=>{result=result+1}; f(); alias=f; alias(); assert result==%d+2`,
		`result=%d; f=()=>{x=result;g=()=>{x=x+2;return x};return g};g=f();result=g();assert result==%d+2`,
		`result=%d;a=[result,1];a[0]=a[0]+2;m={"v":a[0]};m["v"]=m["v"]+1;result=m["v"];assert result==%d+3`,
		`result=%d;a,b=1,2;a,b=b,a;result=result+a+b;assert result==%d+3`,
		`result=%d;a=[1,2];x,y=a;result=result+x+y;assert result==%d+3`,
		`result=%d;ch=make(chan any,1);ch<-result;result=<-ch;assert result==%d`,
		`result=%d;ch=make(chan int);go func(){ch<-result+1}();result=<-ch;assert result==%d+1`,
		`result=%d;f=()=>{defer func(){result=result+3}();return result};f();assert result==%d+3`,
		`result=%d;try{result=result+1;panic("probe")}catch err{result=result+2};assert result==%d+3`,
		`result=%d;f=(a,args...)=>{x=a;for v in args{x=x+v};return x};result=f(result,1,2);assert result==%d+3`,
		`result=%d;value=hostNil();assert value==nil;result=result+1;assert result==%d+1`,
		`result=%d;ch=make(chan any,1);ch<-result;select{case result=<-ch:default:result=99};assert result==%d`,
	} {
		for n := -2; n <= 2; n++ {
			source := fmt.Sprintf(template, n, n)
			t.Run(source, func(t *testing.T) {
				e := New()
				e.ImportLibs(map[string]any{"hostNil": func() any { return nil }})
				codes, err := e.Compile(source)
				require.NoError(t, err)
				states, _, _, _ := snapshotCompiledCodeTemplates(t, codes)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				for repeat := 0; repeat < 2; repeat++ {
					require.NoError(t, e.GetVM().ExecYakCode(ctx, source, codes, yakvm.None))
					require.NoError(t, e.GetVM().AsyncWaitError())
					assertCompiledCodeTemplatesUnchanged(t, states)
				}
			})
		}
	}
}

func TestCoreIncrementalRuntimeOwnership(t *testing.T) {
	e := New()
	const source = `handler=func(n){x=n;for i in 32{x=x+1};a=[x,n];a[0]=a[0]+1;m={"v":a[0]};f=()=>m["v"];alias=f;return alias()}`
	codes, err := e.Compile(source)
	require.NoError(t, err)
	states, _, _, _ := snapshotCompiledCodeTemplates(t, codes)
	require.NoError(t, e.GetVM().ExecYakCode(context.Background(), source, codes, yakvm.None))
	raw, ok := e.GetVar("handler")
	require.True(t, ok)
	f := raw.(*yakvm.Function)
	// The public map and its Values remain caller-owned across local rebinding.
	args := yakvm.YakVMValuesToFunctionMap(f, []*yakvm.Value{yakvm.NewIntValue(5)}, true)
	for i := 0; i < 20; i++ {
		got, err := e.GetVM().ExecYakFunction(context.Background(), f, args, yakvm.None)
		require.NoError(t, err)
		require.Equal(t, 38, got)
		require.Len(t, args, 1)
		for _, value := range args {
			require.Equal(t, 5, value.Value)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := 0; n < 32; n++ {
				input := worker*100 + n
				got, err := e.CallYakFunctionNative(ctx, f, input)
				if err != nil || got != input+33 {
					t.Errorf("input=%d result=%v err=%v", input, got, err)
				}
			}
		}(worker)
	}
	wg.Wait()
	assertCompiledCodeTemplatesUnchanged(t, states)
}

func TestCoreIncrementalRuntimeHandlerSnapshots(t *testing.T) {
	e := New()
	register := func(n int) {
		e.GetVM().RegisterMapMemberCallHandler("test", "value", func(any) any { return func() int { return n } })
	}
	var retained func(int) int
	e.ImportLibs(map[string]any{
		"test":   map[string]any{"value": func() int { return 0 }},
		"update": func() { register(2) },
		"retain": func(fn func(int) int) { retained = fn },
	})
	register(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, e.SafeEvalWithoutCache(ctx, `assert test.value()==1;retain((n)=>test.value()+n);update();assert test.value()==1;nested=()=>test.value();assert nested()==1;ch=make(chan int);go func(){ch<-test.value()}();assert <-ch==1`))
	require.NotNil(t, retained)
	require.Equal(t, 11, retained(10))
	require.NoError(t, e.SafeEvalWithoutCache(ctx, `assert test.value()==2;verify=()=>{a=test.value();for i in 5{assert test.value()==a};return a}`))
	raw, _ := e.GetVar("verify")
	f := raw.(*yakvm.Function)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for n := 0; n < 1000; n++ {
			register(n)
		}
	}()
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 20; n++ {
				if _, err := e.CallYakFunctionNative(ctx, f); err != nil {
					t.Error(err)
				}
				if retained(10) != 11 {
					t.Error("retained callback changed its defining frame's handler snapshot")
				}
			}
		}()
	}
	wg.Wait()
	require.NoError(t, e.SafeEvalWithoutCache(ctx, `assert test.value()==999`))
}

func TestCoreIncrementalRuntimeCancellation(t *testing.T) {
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		e := New()
		e.ImportLibs(map[string]any{"cancelNow": func() int { cancel(); return 7 }})
		err := e.SafeEvalWithoutCache(ctx, `x=1;x=cancelNow();panic("cancel missed")`)
		cancel()
		require.ErrorIs(t, err, context.Canceled)
		value, _ := e.GetVar("x")
		require.Equal(t, 1, value, "cancellation must precede assignment")
	}
	// Synchronize with an executing loop rather than relying on timer timing.
	e := New()
	started := make(chan struct{})
	e.ImportLibs(map[string]any{"started": func() { close(started) }})
	const source = `x=0;for{x=x+1;if x==1000{started()}}`
	codes, err := e.Compile(source)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-started:
			cancel()
		case <-ctx.Done():
		}
	}()
	err = e.GetVM().ExecYakCode(ctx, source, codes, yakvm.None)
	<-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("executing scalar loop failed to observe cancellation: %v", err)
	}
}

func BenchmarkCoreIncrementalExecution(b *testing.B) {
	for _, tc := range []struct{ name, source string }{
		{"arithmetic", `x=0;for i in 1000{x=x+i};assert x==499500`},
		{"functions", `f=(n)=>{x=n;x=x+1;return x};x=0;for i in 1000{x=x+f(i)};assert x==500500`},
		{"native", `x=0;for i in 1000{x=x+host(i)};assert x==500500`},
		{"containers", `x=0;for i in 1000{a=[i,1];a[0]=a[0]+1;m={"v":a[0]};x=x+m["v"]};assert x==500500`},
	} {
		b.Run(tc.name, func(b *testing.B) {
			e := New()
			e.ImportLibs(map[string]any{"host": func(n int) int { return n + 1 }})
			codes, err := e.Compile(tc.source)
			if err != nil {
				b.Fatal(err)
			}
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := e.GetVM().ExecYakCode(ctx, tc.source, codes, yakvm.None); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

package antlr4yak

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestCoreGoExpressionKinds(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         int64
	}{
		{"arithmetic", `go 1+1; assert 6*7 == 42`, 0},
		{"repeated-expression", `for i in 100 { go 1+1 }; assert 6*7 == 42`, 0},
		{"compound", `go mark(1)+mark(10)`, 11},
		{"short-circuit", `go false && mark(100)`, 0},
		{"conditional", `go true ? mark(1) : mark(100)`, 1},
		{"literal", `go func(){ mark(1) }`, 1},
		{"empty-literal", `go func(){}`, 0},
		{"parenthesized-literal", `go ((func(){ mark(1) }))`, 1},
		{"arrow-block", `go () => { mark(1) }`, 1},
		{"arrow-expression", `go () => mark(1)`, 1},
		{"named-definition", `go func task(){ mark(1) }`, 1},
		{"instance", `go fn { mark(1) }`, 1},
		{"parenthesized-call", `go ((mark(1)))`, 1},
		{"returned-closure", `factory=()=>{mark(1); return ()=>mark(100)}; go factory()`, 1},
		{"parenthesized-returned-closure", `factory=()=>{mark(1); return ()=>mark(100)}; go ((factory()))`, 1},
		{"literal-returned-closure", `go func(){mark(1); return ()=>mark(100)}`, 1},
		{"explicit-second-call", `factory=()=>{mark(1); return ()=>mark(100)}; go factory()()`, 101},
		{"function-variable", `f=()=>mark(100); go f`, 0},
		{"function-container", `a=[()=>mark(100)]; go a[0]`, 0},
		{"conditional-function-value", `go true ? (()=>mark(100)) : (()=>mark(200))`, 0},
	} {
		for _, mode := range []string{"source", "formatted", "bytecode"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				var calls atomic.Int64
				e := New()
				e.ImportLibs(map[string]any{"mark": func(n int) int { calls.Add(int64(n)); return n }})
				source := tc.source
				if mode == "formatted" {
					var err error
					source, err = New().FormattedAndSyntaxChecking(source)
					if err != nil {
						t.Fatal(err)
					}
				}
				codes, err := e.Compile(source)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "bytecode" {
					m := yakvm.NewCodesMarshaller()
					data, err := m.Marshal(e.rootSymbol, codes)
					if err != nil {
						t.Fatal(err)
					}
					table, decoded, err := m.Unmarshal(data)
					if err != nil {
						t.Fatal(err)
					}
					e.GetVM().SetSymboltable(table)
					codes = decoded
				}
				if err := e.GetVM().ExecYakCode(context.Background(), source, codes); err != nil {
					t.Fatal(err)
				}
				if err := e.GetVM().AsyncWaitError(); err != nil {
					t.Fatal(err)
				}
				if got := calls.Load(); got != tc.want {
					t.Fatalf("invocation effects: got %d, want %d", got, tc.want)
				}
			})
		}
	}
}

func TestCoreGoExpressionRunsEntirelyInWorker(t *testing.T) {
	for _, source := range []string{
		`go gate()+mark(41)`,
		`start=()=>{value=41; go gate()+mark(value)}; start()`,
		`go func(){ gate(); mark(41) }`,
	} {
		t.Run(source, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			release := make(chan struct{})
			var observed atomic.Int64
			e := New()
			e.ImportLibs(map[string]any{
				"gate": func() int {
					select {
					case <-release:
					case <-ctx.Done():
					}
					return 0
				},
				"mark": func(n int) int { observed.Store(int64(n)); return n },
			})
			done := make(chan error, 1)
			go func() { done <- e.SafeEval(ctx, source) }()
			var evalErr error
			select {
			case evalErr = <-done:
				close(release)
			case <-time.After(2 * time.Second):
				close(release)
				evalErr = <-done
				t.Error("go expression blocked its submitting execution")
			}
			if evalErr != nil {
				t.Fatal(evalErr)
			}
			if err := e.GetVM().AsyncWaitError(); err != nil {
				t.Fatal(err)
			}
			if observed.Load() != 41 {
				t.Fatalf("lost lexical binding: %d", observed.Load())
			}
		})
	}
}

func TestCoreGoCallKeepsArgumentEvaluation(t *testing.T) {
	var arguments atomic.Int64
	values := make(chan int, 1)
	e := New()
	e.ImportLibs(map[string]any{
		"argument": func() int { arguments.Add(1); return 7 },
		"count":    func() int { return int(arguments.Load()) },
		"sink":     func(value int) { values <- value },
	})
	if err := e.SafeEval(context.Background(), `go sink(argument()); assert count()==1`); err != nil {
		t.Fatal(err)
	}
	if err := e.GetVM().AsyncWaitError(); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-values:
		if value != 7 || arguments.Load() != 1 {
			t.Fatalf("argument evaluation changed: %d/%d", value, arguments.Load())
		}
	default:
		t.Fatal("async call did not run")
	}
}

func TestCoreGoChannelExpression(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	e := New()
	e.ImportLibs(map[string]any{"ch": make(chan int)})
	if err := e.SafeEval(ctx, `go ch <- 41; value = <-ch; assert value == 41`); err != nil {
		t.Fatal(err)
	}
	if err := e.GetVM().AsyncWaitError(); err != nil {
		t.Fatal(err)
	}
}

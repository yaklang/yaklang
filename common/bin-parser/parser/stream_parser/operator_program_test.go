package stream_parser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestOperatorProgramCacheConcurrentRuntimeIsolation(t *testing.T) {
	const source = `
if getFromScope("previous") != undefined { panic("previous invocation leaked") }
previous = input
literal = b"abc"
if literal[0] != 97 { panic("mutable compiled literal leaked") }
literal[0] = input
factory = func(base) { return func(delta) { return base + delta } }
sum = func(values...) { total = 0; for v in values { total += v }; return total }
values = [input, 2, 3]
values[1] = input + 1
eval("dynamic = input + 7")
result = factory(input)(sum(values...)) + dynamic
`
	const workers = 24
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(input int) {
			defer wg.Done()
			for repeat := 0; repeat < 3; repeat++ {
				engine := antlr4yak.New()
				engine.ImportLibs(map[string]any{"input": input})
				if err := evalOperatorProgram(context.Background(), engine, source); err != nil {
					errs <- err
					return
				}
				if got := engine.Var("result"); got != 4*input+11 {
					errs <- fmt.Errorf("input %d: got %v, want %d", input, got, 4*input+11)
					return
				}
				if literal, ok := engine.Var("literal").([]byte); !ok || len(literal) != 3 || literal[0] != byte(input) || string(literal[1:]) != "bc" {
					errs <- fmt.Errorf("input %d: compiled byte literal was not isolated: %v", input, engine.Var("literal"))
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func TestOperatorProgramNilAssignmentConcurrentIsolation(t *testing.T) {
	const source = `
missing = nativeNil()
alias = missing
first, err = nativePair()
if missing != nil || alias != nil || err != nil || first != 17 { panic("nil binding changed") }
missing = 42
if alias != nil { panic("nil alias changed") }
result = first + missing
`
	for _, cached := range []bool{false, true} {
		var wg sync.WaitGroup
		errs := make(chan error, 24)
		for worker := 0; worker < 24; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				engine := antlr4yak.New()
				engine.ImportLibs(map[string]any{
					"nativeNil":  func() any { return nil },
					"nativePair": func() (int, error) { return 17, nil },
				})
				var err error
				if cached {
					err = evalOperatorProgram(context.Background(), engine, source)
				} else {
					err = engine.SafeEvalWithoutCache(context.Background(), source)
				}
				if err != nil {
					errs <- err
				} else if engine.Var("result") != 59 {
					errs <- fmt.Errorf("nil assignment result: %v", engine.Var("result"))
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err, "cached=%v", cached)
		}
	}
}

func TestOperatorProgramCacheErrorDoesNotPoisonNextRun(t *testing.T) {
	const source = `if fail { panic("expected operator failure") }; result = input + 1`
	for _, fail := range []bool{false, true, false} {
		engine := antlr4yak.New()
		engine.ImportLibs(map[string]any{"fail": fail, "input": 41})
		err := evalOperatorProgram(context.Background(), engine, source)
		if fail {
			require.ErrorContains(t, err, "expected operator failure")
		} else {
			require.NoError(t, err)
			require.Equal(t, 42, engine.Var("result"))
		}
	}
	for i := 0; i < 2; i++ {
		err := evalOperatorProgram(context.Background(), antlr4yak.New(), `result = (`)
		require.ErrorContains(t, err, "compile error")
	}
}

func TestOperatorProgramCacheIncludesObserveFileChanges(t *testing.T) {
	file := filepath.Join(t.TempDir(), "included.yak")
	source := "include " + strconv.Quote(file)
	for _, value := range []int{17, 29} {
		require.NoError(t, os.WriteFile(file, []byte(fmt.Sprintf("result = %d\n", value)), 0600))
		engine := antlr4yak.New()
		require.NoError(t, evalOperatorProgram(context.Background(), engine, source))
		require.Equal(t, value, engine.Var("result"))
	}
}

func TestOperatorProgramCacheLargeSourceFallback(t *testing.T) {
	// Cache bounds must not introduce a new language/input-size restriction.
	source := "// " + strings.Repeat("x", operatorProgramSourceLimit) + "\nresult = input + 1"
	for _, input := range []int{10, 90} {
		engine := antlr4yak.New()
		engine.ImportLibs(map[string]any{"input": input})
		require.NoError(t, evalOperatorProgram(context.Background(), engine, source))
		require.Equal(t, input+1, engine.Var("result"))
	}
	_, cached := operatorProgramCache.Get(source)
	require.False(t, cached, "oversized source must not be retained by the cache")
}

func TestOperatorProgramCacheLargeArtifactFallback(t *testing.T) {
	// A short source can still compile to an oversized artifact. Verify that
	// the artifact limit bounds retention without rejecting valid operators.
	const increments = 5000
	source := "result = input\n" + strings.Repeat("result += 1\n", increments)
	require.LessOrEqual(t, len(source), operatorProgramSourceLimit)
	compiler := yakast.NewYakCompiler()
	compiler.Compiler(source)
	require.Empty(t, compiler.GetErrors())
	program, err := yakvm.NewCodesMarshaller().Marshal(compiler.GetRootSymbolTable(), compiler.GetOpcodes())
	require.NoError(t, err)
	require.Greater(t, len(program), operatorProgramArtifactLimit)
	for _, input := range []int{13, 91} {
		engine := antlr4yak.New()
		engine.ImportLibs(map[string]any{"input": input})
		require.NoError(t, evalOperatorProgram(context.Background(), engine, source))
		require.Equal(t, input+increments, engine.Var("result"))
		_, cached := operatorProgramCache.Get(source)
		require.False(t, cached, "oversized artifact must not be retained by the cache")
	}
}

func TestOperatorProgramCacheEvictionPreservesResults(t *testing.T) {
	sourceFor := func(value int) string {
		return fmt.Sprintf("// operator-cache-eviction\nresult = input + %d", value)
	}
	run := func(value, input int) {
		engine := antlr4yak.New()
		engine.ImportLibs(map[string]any{"input": input})
		require.NoError(t, evalOperatorProgram(context.Background(), engine, sourceFor(value)))
		require.Equal(t, input+value, engine.Var("result"))
	}
	for value := 0; value <= operatorProgramCacheCapacity; value++ {
		run(value, 7)
	}
	_, oldestPresent := operatorProgramCache.Get(sourceFor(0))
	require.False(t, oldestPresent, "oldest program must be evicted after exceeding capacity")
	_, newestPresent := operatorProgramCache.Get(sourceFor(operatorProgramCacheCapacity))
	require.True(t, newestPresent)
	// An evicted program recompiles correctly with a fresh invocation's input.
	run(0, 101)
	_, reloaded := operatorProgramCache.Get(sourceFor(0))
	require.True(t, reloaded)
}

func BenchmarkOperatorProgram(b *testing.B) {
	const source = `
sum = func(values...) {
    result = 0
    for value in values { result += value }
    return result
}
result = sum(input, 2, 3)
`
	for _, mode := range []string{"compile-every-call", "cached"} {
		b.Run(mode, func(b *testing.B) {
			eval := evalOperatorProgram
			if mode == "compile-every-call" {
				eval = func(ctx context.Context, engine *antlr4yak.Engine, code string) error {
					return engine.SafeEvalWithoutCache(ctx, code)
				}
			}
			ctx := context.Background()
			warm := antlr4yak.New()
			warm.ImportLibs(map[string]any{"input": 7})
			if err := eval(ctx, warm, source); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				engine := antlr4yak.New()
				engine.ImportLibs(map[string]any{"input": 7})
				if err := eval(ctx, engine, source); err != nil {
					b.Fatal(err)
				}
				if got := engine.Var("result"); got != 12 {
					b.Fatalf("got result %v, want 12", got)
				}
			}
		})
	}
}

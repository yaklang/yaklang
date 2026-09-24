package stream_parser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func outProgramTestEngine(libs map[string]any) *antlr4yak.Engine {
	engine := antlr4yak.New()
	engine.GetVM().GetConfig().SetSuppressPanicDebugStack(true)
	engine.ImportLibs(libs)
	return engine
}

func outProgramUncached(engine *antlr4yak.Engine, source string) (any, error) {
	return engine.ExecuteAsExpression(source, nil)
}

func TestOutProgramResultEquivalence(t *testing.T) {
	for _, tc := range []struct {
		name, source string
	}{
		{"empty", ""},
		{"comment-only", "// no stack value\n"},
		{"expression", "input + 1"},
		{"multiple-statements", "answer = input + 1\nanswer * 2"},
		{"assignment-last-stack", "input + 1\nanswer = input + 2"},
		{"return", "answer = input + 1\nreturn answer\npanic(\"unreachable\")"},
		{"branch-return", "if input == 41 { return 73 }; return 99"},
		{"bare-return", "return"},
		{"nil", "return nil"},
		{"undefined", "undefined"},
		{"no-result-call", "nativeVoid()"},
		{"last-call", "input + 1\nnativeValue()"},
		{"loop-last-stack", "answer = 0; for value in [1, 2, 3] { answer += value }; answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, input := range []int{41, 12} {
				libs := map[string]any{"input": input, "nativeVoid": func() {}, "nativeValue": func() int { return input + 3 }}
				want, err := outProgramUncached(outProgramTestEngine(libs), tc.source)
				require.NoError(t, err)
				for repeat := 0; repeat < 2; repeat++ {
					got, err := evalOutProgram(outProgramTestEngine(libs), tc.source)
					require.NoError(t, err)
					require.Equal(t, want, got, "input=%d repeat=%d", input, repeat)
				}
			}
		})
	}
}

func TestOutProgramErrorEquivalenceAndSourceLines(t *testing.T) {
	for _, tc := range []struct {
		name, source, marker string
	}{
		{"compile", "// out compile diagnostic\nvalue = (\n", "compile error"},
		{"panic", "// out panic diagnostic\nvalue = input\nvalue += 1\npanic(\"out-line-four\")\n", "out-line-four"},
		{"nested-panic", "// out nested diagnostic\ninner = func() {\n  value = input\n  panic(\"out-nested-line-four\")\n}\ninner()\n", "out-nested-line-four"},
		{"native-panic", "// out native diagnostic\nvalue = input\nvalue += 1\nnativePanic()\n", "out-native-rejection"},
		{"eval-panic", "// out eval diagnostic\neval(\"panic(\\\"out-eval-rejection\\\")\")", "out-eval-rejection"},
		{"panic-after-eval", "// out after eval diagnostic\neval(\"dynamic = input + 1\")\npanic(\"out-after-eval-rejection\")", "out-after-eval-rejection"},
		{"nested-function-eval-panic", `// out nested eval diagnostic
outer = func() {
  inner = func() { eval("panic(\"out-nested-eval-rejection\")") }
  inner()
}
outer()`, "out-nested-eval-rejection"},
		{"defer-eval-panic", `// out defer eval diagnostic
inner = func() {
  defer func() { eval("panic(\"out-defer-eval-rejection\")") }()
  return 17
}
inner()`, "out-defer-eval-rejection"},
		{"defer-call-after-eval", `// out deferred call diagnostic
defer nativePanic()
eval("dynamic = input + 1")`, "out-native-rejection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			libs := map[string]any{"input": 17, "nativePanic": func() { panic("out-native-rejection") }}
			_, wantErr := outProgramUncached(outProgramTestEngine(libs), tc.source)
			require.ErrorContains(t, wantErr, tc.marker)
			for repeat := 0; repeat < 2; repeat++ {
				got, gotErr := evalOutProgram(outProgramTestEngine(libs), tc.source)
				require.Nil(t, got)
				require.Error(t, gotErr)
				// Compare the complete source excerpt, line numbers and frame names,
				// not merely the panic's final message.
				require.Equal(t, wantErr.Error(), gotErr.Error())
			}
			if tc.name == "compile" {
				_, cached := operatorProgramCache.Get(tc.source)
				require.False(t, cached, "failed compilation must not occupy a cache entry")
			}
		})
	}
}

func TestOutProgramMutableValuesClosuresAndEvalIsolation(t *testing.T) {
	const source = `
if getFromScope("previous") != undefined { panic("previous out invocation leaked") }
previous = input
literal = b"abc"
values = [11, 12, 13]
bag = {"value": 17}
if literal[0] != 97 || values[0] != 11 || bag["value"] != 17 { panic("mutable out value leaked") }
literal[0] = input
values[0] = input + 1
bag["value"] = input + 2
factory = func(base) { return func(delta) { return base + delta } }
eval("dynamic = input + 7")
return factory(input)(values[0] + bag["value"]) + dynamic
`
	for _, eval := range []func(*antlr4yak.Engine, string) (any, error){outProgramUncached, evalOutProgram} {
		for _, input := range []int{7, 23, 7} {
			engine := outProgramTestEngine(map[string]any{"input": input})
			got, err := eval(engine, source)
			require.NoError(t, err)
			require.Equal(t, 4*input+10, got)
			literal, ok := engine.Var("literal").([]byte)
			require.True(t, ok)
			require.Equal(t, []byte{byte(input), 'b', 'c'}, literal)
			// Mutation after execution must not modify the next deserialization.
			literal[1] = 'z'
		}
	}
}

func TestOutProgramNilAliasesAndConcurrentIsolation(t *testing.T) {
	const source = `
if getFromScope("previous") != undefined { panic("concurrent out scope leaked") }
previous = input
missing = nativeNil()
alias = missing
first, err = nativePair()
if missing != nil || alias != nil || err != nil || first != input { panic("nil binding changed") }
missing = input + 1
if alias != nil { panic("nil alias changed") }
literal = b"abc"
if literal[0] != 97 { panic("concurrent mutable out literal leaked") }
literal[0] = input
values = [input, input + 1]
bag = {"value": input + 2}
factory = func(base) { return func(delta) { return base + delta } }
eval("dynamic = input + 3")
return factory(first)(values[1] + bag["value"]) + dynamic
`
	for _, cached := range []bool{false, true} {
		const workers = 12
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for input := 0; input < workers; input++ {
			wg.Add(1)
			go func(input int) {
				defer wg.Done()
				for repeat := 0; repeat < 3; repeat++ {
					engine := outProgramTestEngine(map[string]any{
						"input": input, "nativeNil": func() any { return nil },
						"nativePair": func() (int, error) { return input, nil },
					})
					eval := outProgramUncached
					if cached {
						eval = evalOutProgram
					}
					got, err := eval(engine, source)
					if err != nil {
						errs <- err
						return
					}
					if got != 4*input+6 {
						errs <- fmt.Errorf("cached=%t input=%d: got %v, want %d", cached, input, got, 4*input+6)
						return
					}
				}
			}(input)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
	}
}

func TestOutProgramReturnedClosuresRetainIndependentScopes(t *testing.T) {
	const source = `captured = input; return func(delta) { captured += delta; return captured }`
	for _, eval := range []func(*antlr4yak.Engine, string) (any, error){outProgramUncached, evalOutProgram} {
		engines := []*antlr4yak.Engine{
			outProgramTestEngine(map[string]any{"input": 10}),
			outProgramTestEngine(map[string]any{"input": 100}),
		}
		functions := make([]*yakvm.Function, len(engines))
		for index, engine := range engines {
			got, err := eval(engine, source)
			require.NoError(t, err)
			function, ok := got.(*yakvm.Function)
			require.True(t, ok)
			functions[index] = function
		}
		for _, call := range []struct{ index, delta, want int }{{0, 1, 11}, {1, 2, 102}, {0, 3, 14}, {1, 4, 106}} {
			got, err := engines[call.index].CallYakFunctionNative(context.Background(), functions[call.index], call.delta)
			require.NoError(t, err)
			require.Equal(t, call.want, got)
		}
	}
}

func TestOutProgramFailureDoesNotPoisonNextRun(t *testing.T) {
	const source = `nativeBefore(); if fail { panic("expected out rejection") }; return nativeValue() + 1`
	for _, fail := range []bool{true, false, true, false} {
		beforeCalls, calls := 0, 0
		engine := outProgramTestEngine(map[string]any{
			"nativeBefore": func() { beforeCalls++ },
			"fail":         fail, "nativeValue": func() int { calls++; return 41 },
		})
		got, err := evalOutProgram(engine, source)
		require.Equal(t, 1, beforeCalls, "a runtime error must not replay already executed callbacks")
		if fail {
			require.ErrorContains(t, err, "expected out rejection")
			require.Nil(t, got)
			require.Zero(t, calls, "a runtime error must not retry the expression")
		} else {
			require.NoError(t, err)
			require.Equal(t, 42, got)
			require.Equal(t, 1, calls)
		}
	}
}

func TestOutProgramSharesArtifactsWithOperators(t *testing.T) {
	for _, outFirst := range []bool{false, true} {
		source := fmt.Sprintf("// shared out operator artifact %t\nresult = input + 1; return result * 2", outFirst)
		for index, input := range []int{7, 11, 23} {
			engine := outProgramTestEngine(map[string]any{"input": input})
			if (index%2 == 0) == outFirst {
				got, err := evalOutProgram(engine, source)
				require.NoError(t, err)
				require.Equal(t, 2*(input+1), got)
			} else {
				require.NoError(t, evalOperatorProgram(context.Background(), engine, source))
				require.Equal(t, input+1, engine.Var("result"))
			}
			_, cached := operatorProgramCache.Get(source)
			require.True(t, cached)
		}
	}
}

func TestOutProgramIncludesObserveFileChanges(t *testing.T) {
	file := filepath.Join(t.TempDir(), "out-included.yak")
	source := "include " + strconv.Quote(file) + "\nreturn result"
	for _, value := range []int{17, 29} {
		require.NoError(t, os.WriteFile(file, []byte(fmt.Sprintf("result = %d\n", value)), 0600))
		want, err := outProgramUncached(outProgramTestEngine(nil), source)
		require.NoError(t, err)
		got, err := evalOutProgram(outProgramTestEngine(nil), source)
		require.NoError(t, err)
		require.Equal(t, value, got)
		require.Equal(t, want, got)
		_, cached := operatorProgramCache.Get(source)
		require.False(t, cached)
	}
}

func TestOutProgramLargeSourceFallback(t *testing.T) {
	source := "// out source fallback " + strings.Repeat("x", operatorProgramSourceLimit) + "\nreturn input + 1"
	for _, input := range []int{10, 90} {
		libs := map[string]any{"input": input}
		want, err := outProgramUncached(outProgramTestEngine(libs), source)
		require.NoError(t, err)
		got, err := evalOutProgram(outProgramTestEngine(libs), source)
		require.NoError(t, err)
		require.Equal(t, input+1, got)
		require.Equal(t, want, got)
		_, cached := operatorProgramCache.Get(source)
		require.False(t, cached)
	}
}

func TestOutProgramLargeArtifactFallback(t *testing.T) {
	const increments = 5000
	source := "result = input\n" + strings.Repeat("result += 1\n", increments) + "return result"
	require.LessOrEqual(t, len(source), operatorProgramSourceLimit)
	compiler := yakast.NewYakCompiler()
	compiler.Compiler(source)
	require.Empty(t, compiler.GetErrors())
	program, err := yakvm.NewCodesMarshaller().Marshal(compiler.GetRootSymbolTable(), compiler.GetOpcodes())
	require.NoError(t, err)
	require.Greater(t, len(program), operatorProgramArtifactLimit)
	libs := map[string]any{"input": 13}
	want, err := outProgramUncached(outProgramTestEngine(libs), source)
	require.NoError(t, err)
	got, err := evalOutProgram(outProgramTestEngine(libs), source)
	require.NoError(t, err)
	require.Equal(t, increments+13, got)
	require.Equal(t, want, got)
	_, cached := operatorProgramCache.Get(source)
	require.False(t, cached)
}

func TestOutProgramEvictionPreservesResults(t *testing.T) {
	sourceFor := func(value int) string { return fmt.Sprintf("// out-cache-eviction\nreturn input + %d", value) }
	run := func(value, input int) {
		got, err := evalOutProgram(outProgramTestEngine(map[string]any{"input": input}), sourceFor(value))
		require.NoError(t, err)
		require.Equal(t, input+value, got)
	}
	for value := 0; value < operatorProgramCacheCapacity; value++ {
		run(value, 7)
	}
	// A hit refreshes recency; the next insertion should evict entry 1, not 0.
	run(0, 91)
	run(operatorProgramCacheCapacity, 7)
	_, oldestPresent := operatorProgramCache.Get(sourceFor(1))
	require.False(t, oldestPresent)
	_, refreshedPresent := operatorProgramCache.Get(sourceFor(0))
	require.True(t, refreshedPresent)
	require.LessOrEqual(t, operatorProgramCache.Count(), operatorProgramCacheCapacity)
	run(1, 101)
	_, reloaded := operatorProgramCache.Get(sourceFor(1))
	require.True(t, reloaded)
}

type outProgramSVFixture struct {
	name, source string
	data         *base.NodeValue
	want         any
}

func outProgramSVFixtures(tb testing.TB) []outProgramSVFixture {
	tb.Helper()
	rule, err := os.ReadFile(filepath.Join("..", "..", "rules", "iec61850.yaml"))
	require.NoError(tb, err)
	var document map[string]any
	require.NoError(tb, yaml.Unmarshal(rule, &document))
	sourceFor := func(name string) string {
		definition, ok := document[name].(map[string]any)
		require.True(tb, ok, name)
		source, ok := definition["out"].(string)
		require.True(tb, ok, name)
		require.NotEmpty(tb, source)
		return source
	}
	value := func(v any) *base.NodeValue { return &base.NodeValue{Value: v} }
	number := &base.NodeValue{Name: "Sample Counter", Value: []*base.NodeValue{value(uint8(0x82)), value(uint8(2)), value(uint64(1023))}}
	bytes := &base.NodeValue{Name: "Sample Data", Value: []*base.NodeValue{value(uint8(0x87)), value(uint8(4)), value([]byte{1, 2, 3, 4})}}
	length := &base.NodeValue{Name: "Length", ListValue: true, Value: []*base.NodeValue{value(uint8(0x82)), value(uint8(1)), value(uint8(2))}}
	return []outProgramSVFixture{
		{"IECNumber", sourceFor("IECNumber"), number, uint64(1023)},
		{"IECBytes", sourceFor("IECBytes"), bytes, []byte{1, 2, 3, 4}},
		{"IECLength", sourceFor("IECLength"), length, 258},
	}
}

func outProgramSVEngine(data *base.NodeValue) *antlr4yak.Engine {
	node := &base.Node{Name: data.Name}
	// Keep the actual ExecOut bindings and a fresh default engine in both
	// benchmark paths; only compilation versus artifact reuse differs.
	engine := antlr4yak.New()
	engine.ImportLibs(map[string]any{
		"name": node.Name, "dump": func(any) {},
		"len":  func(v any) int { return reflect.ValueOf(v).Len() },
		"data": data, "node": node,
		"newStructValue": newStructValueAny, "newListValue": newListNodeValue, "newValue": newValueAny,
	})
	return engine
}

func TestOutProgramRealSVExpressions(t *testing.T) {
	fixtures := outProgramSVFixtures(t)
	emptyBytes := fixtures[1]
	emptyBytes.name = "IECBytes-empty"
	emptyBytes.data = &base.NodeValue{Value: []*base.NodeValue{{Value: uint8(0x87)}, {Value: uint8(0)}}}
	emptyBytes.want = ""
	shortLength := fixtures[2]
	shortLength.name = "IECLength-short"
	shortLength.data = &base.NodeValue{ListValue: true, Value: []*base.NodeValue{{Value: uint8(127)}}}
	shortLength.want = uint8(127)
	longLength := fixtures[2]
	longLength.name = "IECLength-four-octets"
	longLength.data = &base.NodeValue{ListValue: true, Value: []*base.NodeValue{
		{Value: uint8(0x84)}, {Value: uint8(0xff)}, {Value: uint8(0xff)}, {Value: uint8(0xff)}, {Value: uint8(0xff)},
	}}
	longLength.want = uint64(0xffffffff)
	fixtures = append(fixtures, emptyBytes, shortLength, longLength)
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			want, err := outProgramUncached(outProgramSVEngine(fixture.data), fixture.source)
			require.NoError(t, err)
			require.EqualValues(t, fixture.want, want)
			for repeat := 0; repeat < 2; repeat++ {
				got, err := evalOutProgram(outProgramSVEngine(fixture.data), fixture.source)
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
		})
	}
}

func TestOutProgramNodeValueBindings(t *testing.T) {
	for _, source := range []string{
		`return data`,
		`return newValue(node, data.Value[2].Value)`,
		`return newStructValue("Object", newValue("field", 42))`,
		`return newListValue(node, newValue("field", 42))`,
	} {
		t.Run(source, func(t *testing.T) {
			data := outProgramSVFixtures(t)[0].data
			oldEngine, newEngine := outProgramSVEngine(data), outProgramSVEngine(data)
			want, err := outProgramUncached(oldEngine, source)
			require.NoError(t, err)
			got, err := evalOutProgram(newEngine, source)
			require.NoError(t, err)
			oldValue, ok := want.(*base.NodeValue)
			require.True(t, ok)
			newValue, ok := got.(*base.NodeValue)
			require.True(t, ok)
			require.Equal(t, oldValue.Name, newValue.Name)
			require.Equal(t, oldValue.ListValue, newValue.ListValue)
			require.Equal(t, oldValue.IsStruct(), newValue.IsStruct())
			if oldValue.IsStruct() || oldValue.IsList() {
				require.Len(t, newValue.Children(), len(oldValue.Children()))
				for index, oldChild := range oldValue.Children() {
					require.Equal(t, oldChild.Name, newValue.Children()[index].Name)
					require.Equal(t, oldChild.Value, newValue.Children()[index].Value)
				}
			} else {
				require.Equal(t, oldValue.Value, newValue.Value)
			}
			if source == "return data" {
				require.Same(t, data, newValue)
			} else if newValue.Origin != nil {
				require.Same(t, newEngine.Var("node"), newValue.Origin)
			}
		})
	}
}

func BenchmarkOutProgramSV(b *testing.B) {
	for _, fixture := range outProgramSVFixtures(b) {
		b.Run(fixture.name, func(b *testing.B) {
			for _, mode := range []string{"compile-every-call", "cached"} {
				b.Run(mode, func(b *testing.B) {
					eval := outProgramUncached
					if mode == "cached" {
						eval = evalOutProgram
					}
					want, err := outProgramUncached(outProgramSVEngine(fixture.data), fixture.source)
					require.NoError(b, err)
					got, err := eval(outProgramSVEngine(fixture.data), fixture.source)
					require.NoError(b, err)
					require.Equal(b, want, got)
					b.ReportAllocs()
					b.ResetTimer()
					for index := 0; index < b.N; index++ {
						got, err := eval(outProgramSVEngine(fixture.data), fixture.source)
						if err != nil || !reflect.DeepEqual(want, got) {
							b.Fatalf("got %v, want %v, error %v", got, want, err)
						}
					}
				})
			}
		})
	}
}

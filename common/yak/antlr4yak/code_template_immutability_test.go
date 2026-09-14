package antlr4yak

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"google.golang.org/protobuf/encoding/protowire"
)

type compiledCodeTemplateState struct {
	code       *yakvm.Code
	opcode     yakvm.OpcodeFlag
	unary      int
	op1        *yakvm.Value
	op2        *yakvm.Value
	op1Type    string
	op1Literal string
	op1Symbol  int
	function   *yakvm.Function
	funcScope  uintptr
	funcFrame  uintptr
	funcBind   string
}

func privateFunctionPointer(t *testing.T, function *yakvm.Function, fieldName string) uintptr {
	t.Helper()
	field := reflect.ValueOf(function).Elem().FieldByName(fieldName)
	require.True(t, field.IsValid(), "Function.%s disappeared", fieldName)
	require.Equal(t, reflect.Ptr, field.Kind(), "Function.%s changed kind", fieldName)
	return field.Pointer()
}

func snapshotCompiledCodeTemplates(t *testing.T, codes []*yakvm.Code) ([]compiledCodeTemplateState, bool, bool, bool) {
	t.Helper()
	states := make([]compiledCodeTemplateState, 0, len(codes))
	seen := make(map[*yakvm.Code]struct{})
	var sawFunctionPush0, sawFunctionPush1, sawEllipsisCall bool

	var walk func([]*yakvm.Code)
	walk = func(current []*yakvm.Code) {
		for index, code := range current {
			if code == nil {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}

			state := compiledCodeTemplateState{
				code:   code,
				opcode: code.Opcode,
				unary:  code.Unary,
				op1:    code.Op1,
				op2:    code.Op2,
			}
			if code.Op1 != nil {
				state.op1Type = code.Op1.TypeVerbose
				state.op1Literal = code.Op1.Literal
				state.op1Symbol = code.Op1.SymbolId
				if function, ok := code.Op1.Value.(*yakvm.Function); ok {
					state.function = function
					state.funcScope = privateFunctionPointer(t, function, "scope")
					state.funcFrame = privateFunctionPointer(t, function, "defineFrame")
					state.funcBind = function.GetBindName()
					if code.Opcode == yakvm.OpPush && code.Unary == 0 {
						sawFunctionPush0 = true
					}
					if code.Opcode == yakvm.OpPush && code.Unary == 1 {
						sawFunctionPush1 = true
					}
					walk(function.GetCodes())
				}
				if nested, ok := code.Op1.Value.([]*yakvm.Code); ok {
					walk(nested)
				}
			}
			states = append(states, state)

			if code.Opcode == yakvm.OpEllipsis && index+1 < len(current) {
				next := current[index+1]
				sawEllipsisCall = next.Opcode == yakvm.OpCall || next.Opcode == yakvm.OpAsyncCall
			}
		}
	}
	walk(codes)
	return states, sawFunctionPush0, sawFunctionPush1, sawEllipsisCall
}

func assertCompiledCodeTemplatesUnchanged(t *testing.T, states []compiledCodeTemplateState) {
	t.Helper()
	for _, state := range states {
		require.Equal(t, state.opcode, state.code.Opcode)
		require.Equal(t, state.unary, state.code.Unary, "compiled unary changed for %s", state.opcode)
		require.Same(t, state.op1, state.code.Op1, "compiled Op1 changed for %s", state.opcode)
		require.Same(t, state.op2, state.code.Op2, "compiled Op2 changed for %s", state.opcode)
		if state.op1 != nil {
			require.Equal(t, state.op1Type, state.code.Op1.TypeVerbose)
			require.Equal(t, state.op1Literal, state.code.Op1.Literal)
			require.Equal(t, state.op1Symbol, state.code.Op1.SymbolId)
		}
		if state.function != nil {
			currentFunction, ok := state.code.Op1.Value.(*yakvm.Function)
			require.True(t, ok)
			require.Same(t, state.function, currentFunction, "compiled Function changed for %s", state.opcode)
			require.Equal(t, state.funcScope, privateFunctionPointer(t, currentFunction, "scope"), "template Function.scope changed")
			require.Equal(t, state.funcFrame, privateFunctionPointer(t, currentFunction, "defineFrame"), "template Function.defineFrame changed")
			require.Equal(t, state.funcBind, currentFunction.GetBindName(), "template Function bind name changed")
		}
	}
}

// marshalCodePayload removes the symbol-table prefix. Symbol names are stored
// from a Go map and may legitimately be emitted in a different order; the
// remaining bytes are the deterministic recursive Code/Function payload.
func marshalCodePayload(t *testing.T, symbolTable *yakvm.SymbolTable, codes []*yakvm.Code) []byte {
	t.Helper()
	raw, err := yakvm.NewCodesMarshaller().Marshal(symbolTable, codes)
	require.NoError(t, err)

	consumeVarint := func() uint64 {
		value, size := protowire.ConsumeVarint(raw)
		require.Positive(t, size, "invalid varint in marshalled symbol table")
		raw = raw[size:]
		return value
	}
	consumeBytes := func() {
		_, size := protowire.ConsumeBytes(raw)
		require.Positive(t, size, "invalid bytes field in marshalled symbol table")
		raw = raw[size:]
	}

	tableCount := int(consumeVarint())
	for tableIndex := 0; tableIndex < tableCount; tableIndex++ {
		consumeVarint() // table index
		consumeBytes()  // verbose
		consumeVarint() // current symbol index
		consumeVarint() // parent index
		childCount := int(consumeVarint())
		for childIndex := 0; childIndex < childCount; childIndex++ {
			consumeVarint()
		}
		symbolCount := int(consumeVarint())
		for symbolIndex := 0; symbolIndex < symbolCount; symbolIndex++ {
			consumeBytes()
			consumeVarint()
		}
	}

	codeCount, size := protowire.ConsumeVarint(raw)
	require.Positive(t, size, "missing marshalled code count")
	require.Equal(t, len(codes), int(codeCount))
	return append([]byte(nil), raw...)
}

func TestCompiledCodeTemplatesRemainImmutableAfterExecution(t *testing.T) {
	const source = `
factory = func(base) {
    captured = func(delta) { return base + delta }
    pure = func() { return 7 }
    return captured(3) + pure()
}
closureResult = factory(10)

sum = func(head, values...) {
    total = head
    for value in values { total += value }
    return total
}
spreadResult = sum(100, [1, 2, 3]...)
`

	engine := New()
	compiler, err := engine._compile(source, engine.rootSymbol)
	require.NoError(t, err)
	codes := compiler.GetOpcodes()
	symbolTable := compiler.GetRootSymbolTable()

	states, sawPush0, sawPush1, sawEllipsisCall := snapshotCompiledCodeTemplates(t, codes)
	require.True(t, sawPush0, "fixture must contain the no-reference Function OpPush path")
	require.True(t, sawPush1, "fixture must contain the Function-copy OpPush path")
	require.True(t, sawEllipsisCall, "fixture must contain Ellipsis followed by Call")
	for _, state := range states {
		if state.function != nil {
			require.Zero(t, state.funcScope, "compiled Function template unexpectedly has a scope")
			require.Zero(t, state.funcFrame, "compiled Function template unexpectedly has a define frame")
		}
	}
	beforePayload := marshalCodePayload(t, symbolTable, codes)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, engine.vm.ExecYakCode(ctx, source, codes, yakvm.None))
	require.Equal(t, 20, engine.Var("closureResult"))
	require.Equal(t, 106, engine.Var("spreadResult"))

	assertCompiledCodeTemplatesUnchanged(t, states)
	afterPayload := marshalCodePayload(t, symbolTable, codes)
	require.Equal(t, beforePayload, afterPayload, "recursive compiled Code payload changed after execution")
}

func TestCachedMarshalOverlapsUnfinishedAsyncCodeSafely(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	const workerCount = 16

	var ready sync.WaitGroup
	ready.Add(workerCount)
	allReady := make(chan struct{})
	go func() {
		ready.Wait()
		close(allReady)
	}()
	start := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(start) })
	var started sync.WaitGroup
	started.Add(workerCount)
	allStarted := make(chan struct{})
	go func() {
		started.Wait()
		close(allStarted)
	}()
	var stop atomic.Bool

	type workerResult struct {
		round int
		value int
	}
	results := make(map[int]workerResult, workerCount)
	var resultsMu sync.Mutex
	engine := New()
	engine.ImportLibs(map[string]interface{}{
		"templateTestReady":     func() { ready.Done() },
		"templateTestWaitStart": func() { <-start },
		"templateTestAwaitReady": func() {
			select {
			case <-allReady:
			case <-time.After(10 * time.Second):
				panic("timed out waiting for Yak async workers")
			}
		},
		"templateTestRelease": func() { releaseOnce.Do(func() { close(start) }) },
		"templateTestStarted": func() { started.Done() },
		"templateTestAwaitStarted": func() {
			select {
			case <-allStarted:
			case <-time.After(10 * time.Second):
				panic("timed out waiting for Yak async workers to start executing")
			}
		},
		"templateTestShouldStop": func() bool { return stop.Load() },
		"templateTestYield":      runtime.Gosched,
		"templateTestEvalSource": func(id int) string {
			return fmt.Sprintf("templateDynamic%d = %d", id, id)
		},
		"templateTestRecord": func(id, round, value int) {
			resultsMu.Lock()
			results[id] = workerResult{round: round, value: value}
			resultsMu.Unlock()
		},
	})

	var source strings.Builder
	fmt.Fprintf(&source, `
workerCount = %d
cases = [[], [1], [1, 2, 3], [1, 2, 3, 4, 5, 6, 7, 8]]

countArgs = func(values...) {
    count = 0
    for value in values { count++ }
    return count
}
invoke = func(values) { return countArgs(100, values...) }
hot = func(id, round) {
    base = id * 1000 + round
    captured = func(delta) { return base + delta }
    pure = func() { return 7 }
    return captured(3) + pure() + invoke(cases[(id + round) %% 4])
}
worker = func(id) {
    templateTestReady()
    templateTestWaitStart()
	// Signal before eval so the top-level frame can return and begin its
	// deferred yakc marshal while these workers are still compiling unique
	// symbols into the shared live SymbolTable.
	templateTestStarted()
	eval(templateTestEvalSource(id))
    round = 0
    last = hot(id, round)
    for {
        if templateTestShouldStop() { break }
        round++
        last = hot(id, round)
        templateTestYield()
    }
    templateTestRecord(id, round, last)
}
for id := 0; id < workerCount; id++ { go worker(id) }
templateTestAwaitReady()
templateTestRelease()
templateTestAwaitStarted()

// Force the deferred cache marshaller to walk a large recursive Function body
// while the released workers above are still executing the shared hot codes.
padding = func() {
`, workerCount)
	for index := 0; index < 1200; index++ {
		fmt.Fprintf(&source, "    paddingValue%d = %d\n", index, index)
	}
	fmt.Fprintf(&source, "    return 0\n}\n// cache-overlap-%s\n", t.TempDir())
	code := source.String()
	require.Greater(t, len(code), YAKC_CACHE_MAX_LENGTH)
	hash := calcHash(code, nil)
	t.Cleanup(func() { yakcCache.Remove(hash) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evalErr := engine.SafeEval(ctx, code)
	stop.Store(true)
	waitDone := make(chan struct{})
	go func() {
		engine.vm.AsyncWait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-ctx.Done():
		t.Fatalf("Yak async workers did not finish: %v", ctx.Err())
	}
	require.NoError(t, evalErr)

	resultsMu.Lock()
	require.Len(t, results, workerCount)
	for id := 0; id < workerCount; id++ {
		result := results[id]
		caseLength := []int{0, 1, 3, 8}[(id+result.round)%4]
		expected := id*1000 + result.round + 11 + caseLength
		require.Equal(t, expected, result.value, "worker %d returned a crossed closure/spread result", id)
	}
	resultsMu.Unlock()

	artifact, ok := HaveYakcCache(code)
	require.True(t, ok, "cache-enabled Eval did not persist its yakc artifact")
	symbolTable, cachedCodes, err := engine.UnMarshal(artifact, nil, code)
	require.NoError(t, err)
	require.NotNil(t, symbolTable)
	require.NotEmpty(t, cachedCodes)
}

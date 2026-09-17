package stream_parser

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func bridgeTestSource(profile string) string {
	return fmt.Sprintf("err = parseMemcachedFields(%q)\nif err != nil { panic(err) }\n", profile)
}

func TestBridgeOperatorClosedGrammar(t *testing.T) {
	for _, call := range []string{
		`parseMemcachedFields("stats-request")`, `parseCassandraFields("options4")`,
		"parseTLS12CertificateAuth(13)", "parseSMTPFields(true)", "parseGQUIC35(false, true)", "parseGSSAPIToken()",
	} {
		require.True(t, reusableBridgeOperator("err = "+call+"\nif err != nil { panic(err) }\n"), call)
	}
	for _, call := range []string{
		`parseMemcachedFields(input)`, `parseMemcachedFields(b"abc")`,
		`parseMemcachedFields("a"+"b")`, `parseMemcachedFields("a\\nb")`,
		`parseMemcachedFields("${eval('x')}")`, `parseMemcachedFields(["x"])`,
		`eval("x = 1")`, `unknown("x")`, `parseHTTP3Stream("data")`,
		`parseGQUIC35(true, false, true)`, `parseGQUIC35(true,)`,
	} {
		require.False(t, reusableBridgeOperator("err = "+call+"\nif err != nil { panic(err) }"), call)
	}
	for _, source := range []string{
		bridgeTestSource("stats-request") + "eval(\"x = 1\")",
		"// include dependency\n" + bridgeTestSource("stats-request"),
		"go " + bridgeTestSource("stats-request"),
		strings.Repeat(" ", bridgeOperatorSourceLimit) + bridgeTestSource("stats-request"),
		strings.Replace(bridgeTestSource("stats-request"), "panic(err)", "save(err)", 1),
	} {
		require.False(t, reusableBridgeOperator(source))
	}
}

func TestBridgeWorkerRetainsEngineAndClearsInvocation(t *testing.T) {
	w := newBridgeWorker()
	engine := w.engine
	source := bridgeTestSource("stats-request")
	p, err := w.program(source)
	require.NoError(t, err)
	firstCode := p.codes[0]
	firstSymbols := p.symbols
	testExactByteFieldsBridgeTransactions(t, []string{"stats-request"}, func(string) []byte {
		return []byte("stats\r\n")
	}, func(n *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
		err := w.execute(context.Background(), source, n, process, []string{ParserMode})
		require.Same(t, engine, w.engine)
		require.Nil(t, w.invocation.node)
		require.Nil(t, w.invocation.operator)
		require.Nil(t, w.invocation.modes)
		require.Nil(t, w.engine.GetVM().CurrentFM())
		require.Equal(t, yakvm.GetUndefined(), w.engine.Var("err"))
		require.Same(t, firstCode, w.programs[0].codes[0])
		require.Same(t, firstSymbols, w.programs[0].symbols)
		return err
	})
}

func TestBridgeWorkerConcurrentBindingsAndNestedLeases(t *testing.T) {
	for worker := 0; worker < 8; worker++ {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			profile := []string{"stats-request", "stats-response", "binary-get-request"}[worker%3]
			testExactByteFieldsBridgeTransactions(t, []string{profile}, func(p string) []byte {
				return memcachedFieldsTestFixtures()[p]
			}, func(n *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
				return ExecOperator(n, bridgeTestSource(profile), func(raw *base.Node) (func(bool), error) {
					// A child lease cannot replace the parent's binding or deadlock
					// on a process-wide single-engine mutex.
					err := ExecOperator(nil, bridgeTestSource("stats-request"), nil, "generate")
					require.ErrorContains(t, err, "structured generation is unsupported")
					return process(raw)
				}, ParserMode)
			})
		})
	}
}

func TestBridgeWorkerErrorsCancellationAndEviction(t *testing.T) {
	w := newBridgeWorker()
	ctx := context.Background()
	for i := 0; i < bridgeWorkerProgramLimit+3; i++ {
		source := bridgeTestSource(fmt.Sprintf("profile-%d", i))
		err := w.execute(ctx, source, nil, nil, []string{"generate"})
		legacy := execFreshOperator(nil, source, nil, []string{"generate"})
		require.Error(t, legacy)
		require.EqualError(t, err, legacy.Error(), "complete Yak source diagnostic must match")
		require.LessOrEqual(t, len(w.programs), bridgeWorkerProgramLimit)
	}
	require.Len(t, w.programs, bridgeWorkerProgramLimit)
	for _, p := range w.programs {
		require.NotEqual(t, bridgeTestSource("profile-0"), p.source)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, w.execute(ctx, bridgeTestSource("stats-request"), nil, nil, []string{"generate"}), context.Canceled)
	require.Nil(t, w.engine.GetVM().CurrentFM())
	// A normal error after cancellation still has the original full location.
	source := bridgeTestSource("profile-0")
	require.EqualError(t, w.execute(context.Background(), source, nil, nil, nil), execFreshOperator(nil, source, nil, nil).Error())
	require.Equal(t, source, w.programs[0].source)
}

func TestBridgeWorkerLeavesDynamicOperatorsIsolated(t *testing.T) {
	const source = `
if getFromScope("previous") != undefined { panic("previous invocation leaked") }
previous = getCtx("input")
literal = b"abc"
if literal[0] != 97 { panic("mutable literal leaked") }
literal[0] = previous
factory = func(base) { return func(delta) { return base + delta } }
eval("dynamic = previous + 7")
setCtx("literal", literal)
setCtx("result", dynamic)
setCtx("closure", factory(previous))
`
	require.False(t, reusableBridgeOperator(source))
	var nodes []*base.Node
	for _, input := range []int{11, 29} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		root.Ctx.SetItem("input", input)
		require.NoError(t, ExecOperator(root, source, nil, ParserMode))
		require.Equal(t, input+7, root.Ctx.GetItem("result"))
		require.Equal(t, []byte{byte(input), 'b', 'c'}, root.Ctx.GetItem("literal"))
		require.IsType(t, &yakvm.Function{}, root.Ctx.GetItem("closure"))
		nodes = append(nodes, root)
	}
	for i, root := range nodes {
		require.NoError(t, ExecOperator(root, `setCtx("result", getCtx("closure")(3))`, nil, ParserMode))
		require.Equal(t, []int{14, 32}[i], root.Ctx.GetItem("result"))
	}
}

func TestBridgeWorkerRuleCoverage(t *testing.T) {
	operators, eligible := 0, 0
	names := make(map[string]bool)
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case yaml.MapSlice:
			for _, item := range v {
				if item.Key == "operator" {
					if source, ok := item.Value.(string); ok {
						operators++
						if reusableBridgeOperator(source) {
							eligible++
							call := strings.TrimPrefix(strings.TrimSpace(source), "err = ")
							name, _, _ := strings.Cut(call, "(")
							names[name] = true
						}
					}
				}
				visit(item.Value)
			}
		case []any:
			for _, item := range v {
				visit(item)
			}
		}
	}
	require.NoError(t, filepath.WalkDir("../../rules", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var doc yaml.MapSlice
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return err
		}
		visit(doc)
		return nil
	}))
	require.True(t, names["parseMemcachedFields"])
	require.True(t, names["parseCassandraFields"])
	require.Greater(t, eligible, 100)
	t.Logf("BRIDGE_WORKER_COVERAGE eligible=%d all_operators=%d native_bindings=%d", eligible, operators, len(names))
}

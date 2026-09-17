package stream_parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/rules"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestOperatorPlanInventory(t *testing.T) {
	counts := map[string]int{}
	var sources []string
	for source := range embeddedOperatorSources() {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		plan, err := loadOperatorPlan(source)
		require.NoError(t, err)
		if plan != nil {
			counts[plan.kind]++
		}
		if strings.HasPrefix(source, "payloadStart = getCurrentPosition()") && strings.Contains(source, "typeNameList") {
			require.NotNil(t, plan, "transport dispatch must compile: %s", source[:200])
		}
	}
	t.Logf("unique embedded operators: %d; declarative plans: %v", len(sources), counts)
	if path := os.Getenv("BIN_PARSER_PLAN_INVENTORY"); path != "" {
		var entries []map[string]string
		files := 0
		require.NoError(t, fs.WalkDir(rules.RuleFS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil
			}
			files++
			wire, err := rules.RuleFS.ReadFile(path)
			require.NoError(t, err)
			var doc yaml.MapSlice
			require.NoError(t, yaml.Unmarshal(wire, &doc))
			var visit func(any, string)
			visit = func(value any, nodePath string) {
				if values, ok := value.(yaml.MapSlice); ok {
					for _, item := range values {
						if item.Key == "operator" {
							source := item.Value.(string)
							plan, err := loadEmbeddedOperatorPlan(source)
							require.NoError(t, err)
							kind := "vm"
							if plan != nil {
								kind = plan.kind
							}
							entries = append(entries, map[string]string{"rule": path, "node": nodePath, "execution": kind, "source": source})
						} else {
							visit(item.Value, nodePath+"/"+fmt.Sprint(item.Key))
						}
					}
				}
			}
			visit(doc, "")
			return nil
		}))
		data, err := json.MarshalIndent(map[string]any{"yaml_files": files, "unique_operators": len(sources), "plan_counts": counts, "operators": entries}, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0600))
	}
}

func TestOperatorPlanSequenceErrorsAndTransactions(t *testing.T) {
	for _, source := range []string{`this.ProcessSubNode("A")`, `this.ProcessSubNode("\u0041")`, "this.ProcessSubNode(\"A\")\nthis.ProcessSubNode(\"B\")\n", "this.ProcessSubNode(\"A\"); this.ProcessSubNode(\"B\")", "this.ProcessSubNode(\"A\")\nthis.ProcessSubNode(\"A\")", "this.ProcessSubNode(\n\"A\"\n)"} {
		plan, err := loadOperatorPlan(source)
		require.NoError(t, err)
		require.NotNil(t, plan)
		for failure := 0; failure < 3; failure++ {
			var effects [2][]string
			var errors [2]error
			for mode := 0; mode < 2; mode++ {
				root := giopBridgeInlineRoot(t, "Package:\n  Parent:\n    A: uint8\n    B: uint8\n")
				require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader([]byte{1, 2}))))
				n := root.Children[0].Children[0]
				calls := 0
				process := func(n *base.Node) (func(bool), error) {
					calls++
					effects[mode] = append(effects[mode], n.Name)
					var err error
					if calls == failure {
						err = fmt.Errorf("failure %s", n.Name)
					}
					return func(recover bool) { effects[mode] = append(effects[mode], fmt.Sprint(recover)) }, err
				}
				if mode == 0 {
					errors[mode] = execFreshOperator(n, source, process, []string{ParserMode})
				} else {
					e := &planExecution{plan: plan, this: convertOperatorNode(n, process)}
					func() {
						defer func() {
							if v := recover(); v != nil {
								errors[mode] = e.diagnostic(v)
							}
						}()
						plan.run(e)
					}()
				}
			}
			require.Equal(t, effects[0], effects[1])
			require.Equal(t, fmt.Sprint(errors[0]), fmt.Sprint(errors[1]))
		}
	}
}

func TestOperatorPlanClosedGrammar(t *testing.T) {
	for _, source := range []string{
		`this.ProcessSubNode(name)`, `this.ProcessSubNode("A"); eval("f()")`,
		`f = this.ProcessSubNode; f("A")`, `go this.ProcessSubNode("A")`,
		`this.ProcessSubNode = this.Result; this.ProcessSubNode("A")`,
		`err = parseMySQLFields(profile); if err != nil { panic(err) }`,
		`err = parseMySQLFields("greeting"); if err != nil { ignore(err) }`,
		`err = parseMQTTFields(3+1); if err != nil { panic(err) }`,
	} {
		p, err := loadOperatorPlan(source)
		require.NoError(t, err)
		require.Nil(t, p, source)
	}
	handled, err := execOperatorPlan(nil, `this.ProcessSubNode("A")`, nil, []string{ParserMode})
	require.False(t, handled)
	require.NoError(t, err)
}

func TestOperatorPlanPinnedCacheAndConcurrentDiagnostics(t *testing.T) {
	const source = `this.ProcessSubNode("A")`
	p, err := loadEmbeddedOperatorPlan(source)
	require.NoError(t, err)
	require.NotNil(t, p)
	for i := 0; i < 520; i++ {
		_, err := loadOperatorPlan(fmt.Sprintf("custom_%d = 1", i))
		require.NoError(t, err)
	}
	pinned, err := loadEmbeddedOperatorPlan(source)
	require.NoError(t, err)
	require.Same(t, p, pinned)
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				e := &planExecution{plan: p}
				e.at(`this.ProcessSubNode("A")`)
				value := fmt.Errorf("worker %d invocation %d", worker, i)
				got, want := e.diagnostic(value), e.vmDiagnostic(value)
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("diagnostic contains another invocation's value: %v", got)
				}
			}
		}(worker)
	}
	wg.Wait()
}

func TestOperatorPlanPortPriorityAndFallback(t *testing.T) {
	p := &portDispatchPlan{
		source:      map[uint16]*portPlanBranch{22: {priority: 0, candidates: []string{"SSH"}}, 80: {priority: 1, candidates: []string{"HTTP"}}, 173: {priority: 3, candidates: []string{"Reply"}}},
		destination: map[uint16]*portPlanBranch{22: {priority: 0, candidates: []string{"SSH"}}, 80: {priority: 1, candidates: []string{"HTTP"}}, 173: {priority: 2, candidates: []string{"Request"}}},
		otherwise:   &portPlanBranch{candidates: []string{"Default"}},
	}
	for _, test := range []struct {
		src, dst uint16
		want     string
	}{{22, 80, "SSH"}, {80, 22, "SSH"}, {173, 173, "Request"}, {173, 0, "Reply"}, {0, 173, "Request"}, {0, 0, "Default"}} {
		require.Equal(t, test.want, p.choose(test.src, test.dst).candidates[0])
	}
	for _, transport := range []string{"TCP", "UDP"} {
		prefix := strings.ReplaceAll(portDispatchPrefix, "$TRANSPORT", transport)
		suffix := portDispatchSuffix
		if transport == "TCP" {
			suffix = strings.Replace(suffix, "debug(op.Message)\n", "", 1)
			suffix = strings.Replace(suffix, "// AFTER_RECOVERY", `debug("parse node %s failed: %v", typeName, op.Message)`, 1)
		}
		source := prefix + "\ntypeNameList = [\"Default\"]\nif src == 7 || dst == 7 { typeNameList = [\"First\"] } else if dst == 8 { eval(\"complex()\") }\n" + suffix
		require.NotNil(t, compilePortDispatchPlan(planTokens(source)), "opaque branch should be retained as VM fallback")
		require.Nil(t, compilePortDispatchPlan(planTokens(source+"\nthis.ProcessSubNode(\"Unexpected\")")), "unrecognized suffix must not be skipped")
	}
}

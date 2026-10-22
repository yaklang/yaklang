package syntaxflow_scan

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestStartScan_SourceRuntimeMatchesStandaloneQueries(t *testing.T) {
	bodies := []string{
		`${*.txt}.pattern_regex(/key/) as $hit
alert $hit`,
		`${*.txt}.pattern_regex(/key/) as $raw
${*.txt}.pattern_regex(/"[^"\n]*"/) as $q
$raw not_inside $q as $hit
alert $hit`,
		`${*.txt}.pattern_regex(/require/) as $ctx
$ctx.regexp(/flag/) as $hit
alert $hit`,
	}
	targets := []struct {
		name  string
		files map[string]string
	}{
		{"left", map[string]string{"a.txt": `"key" key require flag`, "ignored.go": "key"}},
		{"right", map[string]string{"b.txt": "你好key key"}},
	}
	var baseline []string
	for i, body := range bodies {
		content := fmt.Sprintf("desc(mode: \"source\", language: general, title: \"runtime-%d\")\n%s", i, body)
		for _, target := range targets {
			res, err := ssaapi.NewSourceQueryTarget(target.name, target.files).SyntaxFlowWithError(content, ssaapi.QueryWithMemory())
			require.NoError(t, err)
			for _, risk := range res.GetRisks() {
				baseline = append(baseline, fmt.Sprintf("%s:%s:%s:%s", risk.Title, risk.CodeSourceUrl, risk.CodeRange, risk.CodeFragment))
			}
		}
	}
	sort.Strings(baseline)
	require.Len(t, baseline, 8)

	var mu sync.Mutex
	var actual []string
	options := []ssaconfig.Option{
		ssaconfig.WithScanConcurrency(4),
		ssaconfig.WithSyntaxFlowMemory(true),
		WithScanResultCallback(func(batch *ScanResult) {
			if batch == nil || batch.Result == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, risk := range batch.Result.GetRisks() {
				actual = append(actual, fmt.Sprintf("%s:%s:%s:%s", risk.Title, risk.CodeSourceUrl, risk.CodeRange, risk.CodeFragment))
			}
		}),
	}
	for _, target := range targets {
		options = append(options, WithSourceFiles(target.name, target.files))
	}
	for i, body := range bodies {
		options = append(options, ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			RuleName: fmt.Sprintf("source-runtime-%d", i),
			Content:  fmt.Sprintf("desc(mode: \"source\", language: general, title: \"runtime-%d\")\n%s", i, body),
		}))
	}
	require.NoError(t, StartScan(context.Background(), options...))
	sort.Strings(actual)
	require.Equal(t, baseline, actual, "risks, source paths, ranges and fragments must remain identical")
}

func TestStartScan_SourceRuntimeBatchedBranches(t *testing.T) {
	var count int
	err := StartScan(context.Background(),
		WithSourceFiles("batch-branches", map[string]string{"a.txt": strings.Repeat("key ", 1200) + "last"}),
		ssaconfig.WithSyntaxFlowMemory(true),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: general, title: "batch-branches")
${*}.pattern_regex(/key/) as $a
${*}.pattern_regex(/last/) as $b
$a + $b as $hit
alert $hit`,
		}),
		WithScanResultCallback(func(batch *ScanResult) {
			if batch != nil && batch.Result != nil {
				count += batch.Result.RiskCount()
			}
		}),
	)
	require.NoError(t, err)
	require.Equal(t, 1201, count)
}

func TestPrepareSourceRuntimes_PreservesRuleOrderAndTargetIsolation(t *testing.T) {
	var rules []*schema.SyntaxFlowRule
	for _, name := range []string{"first", "second", "third"} {
		frame, err := sfvm.CompileRule("desc(mode: source, language: general)\n${*}.pattern_regex(/key/) as $hit\nalert $hit")
		require.NoError(t, err)
		rule := frame.GetRule()
		rule.RuleName = name
		rules = append(rules, rule)
	}
	left := ssaapi.NewSourceQueryTarget("left", map[string]string{"left.txt": "key key"})
	right := ssaapi.NewSourceQueryTarget("right", map[string]string{"right.txt": "key"})
	manager := &scanManager{
		ctx: context.Background(),
		Config: &Config{ScanTaskCallback: &ScanTaskCallback{
			QueryTargets: []ssaapi.SyntaxFlowQueryInstance{left, right},
		}},
		ruleChan: make(chan *schema.SyntaxFlowRule, len(rules)),
	}
	for _, rule := range rules {
		manager.ruleChan <- rule
	}
	close(manager.ruleChan)
	runtimes, cleanup := manager.prepareSourceRuntimes()
	defer cleanup()
	require.Len(t, runtimes, 2)
	require.NoError(t, runtimes[left].Wait(context.Background()))
	require.NoError(t, runtimes[right].Wait(context.Background()))
	require.Equal(t, 2, runtimes[left].Stats().Hits)
	require.Equal(t, 1, runtimes[right].Stats().Hits)
	var restored []*schema.SyntaxFlowRule
	for rule := range manager.ruleChan {
		restored = append(restored, rule)
	}
	require.Equal(t, rules, restored, "resume task indices depend on original rule order")
}

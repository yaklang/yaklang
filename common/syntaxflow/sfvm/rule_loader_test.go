package sfvm

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestCompileRulesOrderAndErrors(t *testing.T) {
	texts := []string{`desc(mode: "source", title: "first") ${*}.regexp(/key/) as $x`, "", `desc(title: "last") * as $x`}
	rules, errs := CompileRules(texts)
	require.Len(t, rules, 3)
	require.NoError(t, errs[0])
	require.Error(t, errs[1])
	require.Nil(t, rules[1])
	require.NoError(t, errs[2])
	require.Equal(t, "first", rules[0].Title)
	require.Equal(t, "last", rules[2].Title)
	require.NotEmpty(t, rules[0].OpCodes)
	for _, rule := range []*schema.SyntaxFlowRule{rules[0], rules[2]} {
		vm := NewSyntaxFlowVirtualMachine()
		_, resave, err := vm.Load(rule)
		require.NoError(t, err)
		require.False(t, resave)
		require.Zero(t, vm.CompileCount())
	}
}

func TestRuleLoaderConcurrentColdLoadAndFreshFrames(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: `desc(title: "loader") * as $x`}
	loader := &RuleLoader{}
	const n = 24
	frames := make([]*SFFrame, n)
	counts := make([]int64, n)
	resaves := make([]bool, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range frames {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			vm := NewSyntaxFlowVirtualMachine()
			frames[i], resaves[i], errs[i] = loader.Load(vm, rule)
			counts[i] = vm.CompileCount()
		}(i)
	}
	wg.Wait()
	var count, saved int64
	for i, frame := range frames {
		require.NoError(t, errs[i])
		count += counts[i]
		if resaves[i] {
			saved++
		}
		if i > 0 {
			require.NotSame(t, frames[0], frame)
			require.NotSame(t, frames[0].config, frame.config)
			require.Same(t, frames[0].Codes[0], frame.Codes[0], "only instructions are shared")
		}
	}
	require.EqualValues(t, 1, count)
	require.EqualValues(t, 1, saved)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	WithContext(ctx)(frames[0].config)
	require.ErrorIs(t, frames[0].GetContext().Err(), context.Canceled)
	require.NoError(t, frames[1].GetContext().Err())
}

func TestRuleLoaderInvalidatesChangedContent(t *testing.T) {
	loader := &RuleLoader{}
	rule := &schema.SyntaxFlowRule{Content: "* as $old"}
	_, _, err := loader.Load(NewSyntaxFlowVirtualMachine(), rule)
	require.NoError(t, err)
	old := rule.OpCodes
	rule.Content = "* as $new"
	vm := NewSyntaxFlowVirtualMachine()
	frame, resave, err := loader.Load(vm, rule)
	require.NoError(t, err)
	require.True(t, resave)
	require.EqualValues(t, 1, vm.CompileCount())
	require.NotEqual(t, old, rule.OpCodes)
	require.Equal(t, rule.Content, frame.Text)
}

func BenchmarkRuleLoader(b *testing.B) {
	frame, err := CompileRule(`desc(mode: "source", title: "loader bench")
${*.php}.pattern_regex(/(key|token)\s*=\s*["']/) as $hit
alert $hit`)
	if err != nil {
		b.Fatal(err)
	}
	rule := frame.GetRule()
	b.Run("decode_each_load", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := NewSyntaxFlowVirtualMachine().Load(rule); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("scan_local_loader", func(b *testing.B) {
		loader := &RuleLoader{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := loader.Load(NewSyntaxFlowVirtualMachine(), rule); err != nil {
				b.Fatal(err)
			}
		}
	})
}

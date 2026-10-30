package sfdb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func includeCycleRule(t *testing.T, content, ruleName string) *schema.SyntaxFlowRule {
	t.Helper()
	rule, err := CheckSyntaxFlowRuleContent(content)
	require.NoError(t, err)
	rule.RuleName = ruleName
	return rule
}

// TestCheckIncludeCycles covers the internal-rule-set dependency analysis:
// the whole supplied set is keyed by title / included_name, and a cycle in
// the resulting <include(...)> graph is rejected with its path.
func TestCheckIncludeCycles(t *testing.T) {
	t.Run("detects mutual cycle", func(t *testing.T) {
		err := CheckIncludeCycles([]*schema.SyntaxFlowRule{
			includeCycleRule(t, `desc(lib: "a"); <include("b")> as $o; alert $o;`, "lib-a"),
			includeCycleRule(t, `desc(lib: "b"); <include("a")> as $o; alert $o;`, "lib-b"),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "include cycle")
		require.Contains(t, err.Error(), "a -> b -> a")
	})

	t.Run("detects self include", func(t *testing.T) {
		err := CheckIncludeCycles([]*schema.SyntaxFlowRule{
			includeCycleRule(t, `desc(lib: "self"); <include("self")> as $o; alert $o;`, "lib-self"),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "self -> self")
	})

	t.Run("detects indirect cycle", func(t *testing.T) {
		err := CheckIncludeCycles([]*schema.SyntaxFlowRule{
			includeCycleRule(t, `desc(lib: "a"); <include("b")> as $o; alert $o;`, "lib-a"),
			includeCycleRule(t, `desc(lib: "b"); <include("c")> as $o; alert $o;`, "lib-b"),
			includeCycleRule(t, `desc(lib: "c"); <include("a")> as $o; alert $o;`, "lib-c"),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "a -> b -> c -> a")
	})

	t.Run("allows acyclic rule set", func(t *testing.T) {
		rules := []*schema.SyntaxFlowRule{
			includeCycleRule(t, `desc(lib: "a"); <include("b")> as $o; alert $o;`, "lib-a"),
			includeCycleRule(t, `desc(lib: "b"); <include("c")> as $o; alert $o;`, "lib-b"),
			includeCycleRule(t, `desc(lib: "c"); a* as $x; alert $x;`, "lib-c"),
			// diamond: two independent edges into the same chain
			includeCycleRule(t, `desc(lib: "d"); <include("b")> as $one; <include("c")> as $two; alert $one;`, "lib-d"),
			// target outside the set has no known content and is skipped
			includeCycleRule(t, `desc(lib: "e"); <include("missing-library")> as $o; alert $o;`, "lib-e"),
		}
		require.NoError(t, CheckIncludeCycles(rules))
	})

	t.Run("detects cycle through embedded sub-rule", func(t *testing.T) {
		err := CheckIncludeCycles([]*schema.SyntaxFlowRule{
			includeCycleRule(t, `desc(lib: "a"); <include("b")> as $o; alert $o;`, "lib-a"),
			includeCycleRule(t, "desc(lib: \"b\");\na* as $x; $x-{ check: `<include(a)> as $o` }-> as $out; alert $out;", "lib-b"),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "a -> b -> a")
	})

	t.Run("skips uncompilable rule", func(t *testing.T) {
		require.NoError(t, CheckIncludeCycles([]*schema.SyntaxFlowRule{
			{Title: "broken", Content: "((("},
			includeCycleRule(t, `desc(lib: "a"); <include("b")> as $o; alert $o;`, "lib-a"),
			includeCycleRule(t, `desc(lib: "b"); a* as $x; alert $x;`, "lib-b"),
		}))
	})

	t.Run("non-library rules are ignored", func(t *testing.T) {
		// Only libraries can sit on an include cycle in the internal rule set;
		// a plain scanning rule is not a graph node, and an edge into it is
		// skipped.
		require.NoError(t, CheckIncludeCycles([]*schema.SyntaxFlowRule{
			includeCycleRule(t, `desc(title: "x"); <include("y")> as $o; alert $o;`, "rule-x"),
			includeCycleRule(t, `desc(lib: "y"); <include("x")> as $o; alert $o;`, "lib-y"),
		}))
	})

	t.Run("empty sets pass", func(t *testing.T) {
		require.NoError(t, CheckIncludeCycles(nil))
		require.NoError(t, CheckIncludeCycles([]*schema.SyntaxFlowRule{nil}))
	})
}

func mustCompileRuleForInclude(t *testing.T, content string) *sfvm.SFFrame {
	t.Helper()
	frame, err := sfvm.CompileRule(content)
	if err != nil {
		t.Fatalf("compile %q: %v", content, err)
	}
	return frame
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("targets mismatch: got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("targets mismatch: got %v want %v", got, want)
		}
	}
}

// TestExtractIncludeTargets covers the compiled forms an include call can
// take: quoted literal, bare identifier (its source text), named params
// beating positional, runtime precedence when the winning named param is
// empty, multiple calls in one rule, and rules without any include.
func TestExtractIncludeTargets(t *testing.T) {
	assertStringSliceEqual(t,
		extractIncludeTargets(mustCompileRuleForInclude(t, `<include("lib-a")> as $out; alert $out;`)),
		[]string{"lib-a"},
	)
	// A bare identifier arg is reduced to its source text at compile time —
	// the same string runtime resolution looks up.
	assertStringSliceEqual(t,
		extractIncludeTargets(mustCompileRuleForInclude(t, `<include(lib_b)> as $out; alert $out;`)),
		[]string{"lib_b"},
	)
	assertStringSliceEqual(t,
		extractIncludeTargets(mustCompileRuleForInclude(t, `<include("positional", name: "named")> as $out; alert $out;`)),
		[]string{"named"},
	)
	// name beats rule beats rulename, first present key wins regardless of
	// value — mirror of NativeCallActualParams.GetString key order.
	assertStringSliceEqual(t,
		extractIncludeTargets(mustCompileRuleForInclude(t, `<include(rule: "by-rule", name: "")> as $out; alert $out;`)),
		[]string(nil),
	)
	// Empty winning named param falls through to the positional param.
	frame := &sfvm.SFFrame{Codes: []*sfvm.SFI{{
		OpCode:   sfvm.OpNativeCall,
		UnaryStr: includeNativeCallName,
		SyntaxFlowConfig: []*sfvm.RecursiveConfigItem{
			{Key: "name", Value: ""},
			{Key: "0", Value: "fallback"},
		},
	}}}
	assertStringSliceEqual(t, extractIncludeTargets(frame), []string{"fallback"})

	assertStringSliceEqual(t,
		extractIncludeTargets(mustCompileRuleForInclude(t, `
a* as $check;
$check<include("first")> as $one;
$one<include("second")> as $two;
alert $two;
`)),
		[]string{"first", "second"},
	)
	assertStringSliceEqual(t,
		extractIncludeTargets(mustCompileRuleForInclude(t, `a* as $x; alert $x;`)),
		nil,
	)
	if extractIncludeTargets(nil) != nil {
		t.Fatal("nil frame must yield no targets")
	}
}

// TestExtractIncludeTargetsWalksEmbeddedSubRules covers include calls inside
// config values that are themselves filter statements (SyntaxFlowRule items):
// extracted through real syntax, only walked when the compile-time flag is
// set, and a fragment that fails to compile standalone is skipped.
func TestExtractIncludeTargetsWalksEmbeddedSubRules(t *testing.T) {
	// Real syntax: a deep-next filter whose config value is a backtick filter
	// statement containing the include.
	assertStringSliceEqual(t,
		extractIncludeTargets(mustCompileRuleForInclude(t,
			"a* as $x; $x-{ check: `<include(embedded_lib)> as $o` }-> as $out; alert $out;")),
		[]string{"embedded_lib"},
	)

	frame := &sfvm.SFFrame{Codes: []*sfvm.SFI{{
		OpCode: sfvm.OpGetBottomUsers,
		SyntaxFlowConfig: []*sfvm.RecursiveConfigItem{
			{Key: "check", Value: `<include("synthetic-embedded")> as $o; alert $o;`, SyntaxFlowRule: true},
			// same textual shape without the flag: a plain value, not walked
			{Key: "plain", Value: `<include("not-walked")> as $o; alert $o;`},
			// unparseable fragment contributes nothing and must not error out
			{Key: "broken", Value: "(((", SyntaxFlowRule: true},
		},
	}}}
	assertStringSliceEqual(t, extractIncludeTargets(frame), []string{"synthetic-embedded"})
}

package sfdb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

func cycleGateLib(t *testing.T, content, ruleName string) *schema.SyntaxFlowRule {
	t.Helper()
	rule, err := CheckSyntaxFlowRuleContent(content)
	require.NoError(t, err)
	rule.RuleName = ruleName
	return rule
}

// TestCheckIncludeCycleForWriteDB exercises the write-time include-cycle gate
// against an isolated database: a candidate closing a cycle is rejected, an
// acyclic one passes, the candidate overlays the stored same-named rule
// (update semantics), and plain rules join the graph by title the way
// GetLibrary resolves them at runtime.
func TestCheckIncludeCycleForWriteDB(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowRule{}).Error)

	a := cycleGateLib(t, "desc(lib: \"a\");\n$input<include(\"b\")> as $o; alert $o;", "cycle-lib-a")
	require.NoError(t, db.Create(a).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Where("rule_name = ?", "cycle-lib-a").Unscoped().Delete(&schema.SyntaxFlowRule{}).Error)
	})

	t.Run("rejects closing a cycle", func(t *testing.T) {
		b := cycleGateLib(t, "desc(lib: \"b\");\n$input<include(\"a\")> as $o; alert $o;", "cycle-lib-b")
		err := CheckIncludeCycleForWriteDB(db, b)
		require.Error(t, err)
		require.Contains(t, err.Error(), "a -> b -> a")
	})

	t.Run("allows acyclic candidate", func(t *testing.T) {
		c := cycleGateLib(t, "desc(lib: \"c\");\n$input<include(\"a\")> as $o; alert $o;", "cycle-lib-c")
		require.NoError(t, CheckIncludeCycleForWriteDB(db, c))
	})

	t.Run("candidate overlays the stored same-named rule", func(t *testing.T) {
		b := cycleGateLib(t, "desc(lib: \"b\");\n$input<include(\"a\")> as $o; alert $o;", "cycle-lib-b")
		require.NoError(t, db.Create(b).Error)
		t.Cleanup(func() {
			require.NoError(t, db.Where("rule_name = ?", "cycle-lib-b").Unscoped().Delete(&schema.SyntaxFlowRule{}).Error)
		})
		// The stored graph is cyclic (a -> b -> a), but the candidate replaces
		// a's node with an include-free version, so the write must pass.
		a2 := cycleGateLib(t, "desc(lib: \"a\");\na* as $o; alert $o;", "cycle-lib-a")
		require.NoError(t, CheckIncludeCycleForWriteDB(db, a2))
	})

	t.Run("non-library title joins the graph", func(t *testing.T) {
		// GetLibrary resolves by title too, so a plain rule titled "x" is a
		// cycle node even without allow-included.
		x := cycleGateLib(t, "desc(title: \"x\");\n$input<include(\"y\")> as $o; alert $o;", "cycle-rule-x")
		require.NoError(t, db.Create(x).Error)
		t.Cleanup(func() {
			require.NoError(t, db.Where("rule_name = ?", "cycle-rule-x").Unscoped().Delete(&schema.SyntaxFlowRule{}).Error)
		})

		y := cycleGateLib(t, "desc(lib: \"y\");\n$input<include(\"x\")> as $o; alert $o;", "cycle-lib-y")
		err := CheckIncludeCycleForWriteDB(db, y)
		require.Error(t, err)
		require.Contains(t, err.Error(), "x -> y -> x")
	})

	require.NoError(t, CheckIncludeCycleForWriteDB(db, nil))
}

func includeCycleLib(content string) *schema.SyntaxFlowRule {
	return &schema.SyntaxFlowRule{Content: content}
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

func TestCheckRuleIncludeCyclesDetectsMutualCycle(t *testing.T) {
	err := checkRuleIncludeCycles(map[string]*schema.SyntaxFlowRule{
		"a": includeCycleLib(`<include("b")> as $out; alert $out;`),
		"b": includeCycleLib(`<include("a")> as $out; alert $out;`),
	})
	if err == nil {
		t.Fatal("mutual include rule set unexpectedly accepted")
	}
	if msg := err.Error(); !strings.Contains(msg, "include cycle") || !strings.Contains(msg, "a -> b -> a") {
		t.Fatalf("cycle error does not name the path: %v", msg)
	}
}

func TestCheckRuleIncludeCyclesDetectsSelfInclude(t *testing.T) {
	err := checkRuleIncludeCycles(map[string]*schema.SyntaxFlowRule{
		"self": includeCycleLib(`<include("self")> as $out; alert $out;`),
	})
	if err == nil {
		t.Fatal("self-including rule set unexpectedly accepted")
	}
	if msg := err.Error(); !strings.Contains(msg, "self -> self") {
		t.Fatalf("cycle error does not name the path: %v", msg)
	}
}

func TestCheckRuleIncludeCyclesDetectsIndirectCycle(t *testing.T) {
	err := checkRuleIncludeCycles(map[string]*schema.SyntaxFlowRule{
		"a": includeCycleLib(`<include("b")> as $out; alert $out;`),
		"b": includeCycleLib(`<include("c")> as $out; alert $out;`),
		"c": includeCycleLib(`<include("a")> as $out; alert $out;`),
	})
	if err == nil {
		t.Fatal("indirect cycle rule set unexpectedly accepted")
	}
	if msg := err.Error(); !strings.Contains(msg, "a -> b -> c -> a") {
		t.Fatalf("cycle error does not name the path: %v", msg)
	}
}

// TestCheckRuleIncludeCyclesAllowsAcyclicRuleSet covers the shapes that must
// NOT trip the check: chains, diamonds, edges to libraries outside the set
// (no known content — skipped), and an empty set.
func TestCheckRuleIncludeCyclesAllowsAcyclicRuleSet(t *testing.T) {
	ruleSet := map[string]*schema.SyntaxFlowRule{
		"a": includeCycleLib(`<include("b")> as $out; alert $out;`),
		"b": includeCycleLib(`<include("c")> as $out; alert $out;`),
		"c": includeCycleLib(`a* as $x; alert $x;`),
		// diamond: two independent edges into the chain
		"d": includeCycleLib(`
<include("b")> as $one;
<include("c")> as $two;
alert $one;
`),
		// target outside the set cannot close a cycle here
		"e": includeCycleLib(`<include("missing-library")> as $out; alert $out;`),
	}
	if err := checkRuleIncludeCycles(ruleSet); err != nil {
		t.Fatalf("acyclic rule set rejected: %v", err)
	}
	if err := checkRuleIncludeCycles(nil); err != nil {
		t.Fatalf("empty rule set rejected: %v", err)
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

func TestCheckRuleIncludeCyclesDetectsCycleThroughEmbeddedSubRule(t *testing.T) {
	err := checkRuleIncludeCycles(map[string]*schema.SyntaxFlowRule{
		"a": includeCycleLib("a* as $x; $x-{ check: `<include(b)> as $o` }-> as $out; alert $out;"),
		"b": includeCycleLib(`<include("a")> as $out; alert $out;`),
	})
	if err == nil {
		t.Fatal("cycle through embedded sub-rule unexpectedly accepted")
	}
	if msg := err.Error(); !strings.Contains(msg, "a -> b -> a") {
		t.Fatalf("cycle error does not name the path: %v", msg)
	}
}

// TestCheckRuleIncludeCyclesSkipsUncompilableRules: a rule that cannot
// compile cannot execute includes at scan time either, so it contributes no
// edges — and must not fail the whole check (one legacy broken row would
// otherwise block every later write through the gate).
func TestCheckRuleIncludeCyclesSkipsUncompilableRules(t *testing.T) {
	err := checkRuleIncludeCycles(map[string]*schema.SyntaxFlowRule{
		"broken": includeCycleLib("((("),
		"a":      includeCycleLib(`<include("b")> as $out; alert $out;`),
		"b":      includeCycleLib(`a* as $x; alert $x;`),
	})
	if err != nil {
		t.Fatalf("rule set with uncompilable rule rejected: %v", err)
	}
}

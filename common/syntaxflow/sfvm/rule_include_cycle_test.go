package sfvm

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

func includeCycleLib(content string) *schema.SyntaxFlowRule {
	return &schema.SyntaxFlowRule{Content: content}
}

func mustCompileRuleForInclude(t *testing.T, content string) *SFFrame {
	t.Helper()
	frame, err := CompileRule(content)
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
		ExtractIncludeTargets(mustCompileRuleForInclude(t, `<include("lib-a")> as $out; alert $out;`)),
		[]string{"lib-a"},
	)
	// A bare identifier arg is reduced to its source text at compile time —
	// the same string runtime resolution looks up.
	assertStringSliceEqual(t,
		ExtractIncludeTargets(mustCompileRuleForInclude(t, `<include(lib_b)> as $out; alert $out;`)),
		[]string{"lib_b"},
	)
	assertStringSliceEqual(t,
		ExtractIncludeTargets(mustCompileRuleForInclude(t, `<include("positional", name: "named")> as $out; alert $out;`)),
		[]string{"named"},
	)
	// name beats rule beats rulename, first present key wins regardless of
	// value — mirror of NativeCallActualParams.GetString key order.
	assertStringSliceEqual(t,
		ExtractIncludeTargets(mustCompileRuleForInclude(t, `<include(rule: "by-rule", name: "")> as $out; alert $out;`)),
		[]string(nil),
	)
	// Empty winning named param falls through to the positional param.
	frame := &SFFrame{Codes: []*SFI{{
		OpCode:   OpNativeCall,
		UnaryStr: includeNativeCallName,
		SyntaxFlowConfig: []*RecursiveConfigItem{
			{Key: "name", Value: ""},
			{Key: "0", Value: "fallback"},
		},
	}}}
	assertStringSliceEqual(t, ExtractIncludeTargets(frame), []string{"fallback"})

	assertStringSliceEqual(t,
		ExtractIncludeTargets(mustCompileRuleForInclude(t, `
	a* as $check;
	$check<include("first")> as $one;
	$one<include("second")> as $two;
	alert $two;
	`)),
		[]string{"first", "second"},
	)
	assertStringSliceEqual(t,
		ExtractIncludeTargets(mustCompileRuleForInclude(t, `a* as $x; alert $x;`)),
		nil,
	)
	if ExtractIncludeTargets(nil) != nil {
		t.Fatal("nil frame must yield no targets")
	}
}

func TestCheckRuleIncludeCyclesDetectsMutualCycle(t *testing.T) {
	err := CheckRuleIncludeCycles(map[string]*schema.SyntaxFlowRule{
		"a": includeCycleLib(`<include("b")> as $out; alert $out;`),
		"b": includeCycleLib(`<include("a")> as $out; alert $out;`),
	})
	if err == nil {
		t.Fatal("mutual include snapshot unexpectedly accepted")
	}
	if msg := err.Error(); !strings.Contains(msg, "include cycle") || !strings.Contains(msg, "a -> b -> a") {
		t.Fatalf("cycle error does not name the path: %v", msg)
	}
}

func TestCheckRuleIncludeCyclesDetectsSelfInclude(t *testing.T) {
	err := CheckRuleIncludeCycles(map[string]*schema.SyntaxFlowRule{
		"self": includeCycleLib(`<include("self")> as $out; alert $out;`),
	})
	if err == nil {
		t.Fatal("self-including snapshot unexpectedly accepted")
	}
	if msg := err.Error(); !strings.Contains(msg, "self -> self") {
		t.Fatalf("cycle error does not name the path: %v", msg)
	}
}

func TestCheckRuleIncludeCyclesDetectsIndirectCycle(t *testing.T) {
	err := CheckRuleIncludeCycles(map[string]*schema.SyntaxFlowRule{
		"a": includeCycleLib(`<include("b")> as $out; alert $out;`),
		"b": includeCycleLib(`<include("c")> as $out; alert $out;`),
		"c": includeCycleLib(`<include("a")> as $out; alert $out;`),
	})
	if err == nil {
		t.Fatal("indirect cycle snapshot unexpectedly accepted")
	}
	if msg := err.Error(); !strings.Contains(msg, "a -> b -> c -> a") {
		t.Fatalf("cycle error does not name the path: %v", msg)
	}
}

// TestCheckRuleIncludeCyclesAllowsAcyclicSnapshot covers the shapes that must
// NOT trip the check: chains, diamonds, edges to libraries outside the
// snapshot (no known content — skipped), and an empty snapshot.
func TestCheckRuleIncludeCyclesAllowsAcyclicSnapshot(t *testing.T) {
	snapshot := map[string]*schema.SyntaxFlowRule{
		"a": includeCycleLib(`<include("b")> as $out; alert $out;`),
		"b": includeCycleLib(`<include("c")> as $out; alert $out;`),
		"c": includeCycleLib(`a* as $x; alert $x;`),
		// diamond: two independent edges into the chain
		"d": includeCycleLib(`
<include("b")> as $one;
<include("c")> as $two;
alert $one;
`),
		// target outside the snapshot cannot close a cycle here
		"e": includeCycleLib(`<include("missing-library")> as $out; alert $out;`),
	}
	if err := CheckRuleIncludeCycles(snapshot); err != nil {
		t.Fatalf("acyclic snapshot rejected: %v", err)
	}
	if err := CheckRuleIncludeCycles(nil); err != nil {
		t.Fatalf("empty snapshot rejected: %v", err)
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
		ExtractIncludeTargets(mustCompileRuleForInclude(t,
			"a* as $x; $x-{ check: `<include(embedded_lib)> as $o` }-> as $out; alert $out;")),
		[]string{"embedded_lib"},
	)

	frame := &SFFrame{Codes: []*SFI{{
		OpCode: OpGetBottomUsers,
		SyntaxFlowConfig: []*RecursiveConfigItem{
			{Key: "check", Value: `<include("synthetic-embedded")> as $o; alert $o;`, SyntaxFlowRule: true},
			// same textual shape without the flag: a plain value, not walked
			{Key: "plain", Value: `<include("not-walked")> as $o; alert $o;`},
			// unparseable fragment contributes nothing and must not error out
			{Key: "broken", Value: "(((", SyntaxFlowRule: true},
		},
	}}}
	assertStringSliceEqual(t, ExtractIncludeTargets(frame), []string{"synthetic-embedded"})
}

func TestCheckRuleIncludeCyclesDetectsCycleThroughEmbeddedSubRule(t *testing.T) {
	err := CheckRuleIncludeCycles(map[string]*schema.SyntaxFlowRule{
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
	err := CheckRuleIncludeCycles(map[string]*schema.SyntaxFlowRule{
		"broken": includeCycleLib("((("),
		"a":      includeCycleLib(`<include("b")> as $out; alert $out;`),
		"b":      includeCycleLib(`a* as $x; alert $x;`),
	})
	if err != nil {
		t.Fatalf("snapshot with uncompilable rule rejected: %v", err)
	}
}

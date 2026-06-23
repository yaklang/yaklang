package tests

import (
	"embed"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

// TestDecompileSwitchCaseOrder guards against the switch case-mapping corruption caused by the
// invalid sort.Slice comparators (always returning true) that scrambled the switch successor order
// at both the opcode level (CalcOpcodeStackInfo) and the rewriter level (SwitchRewriter). The bug
// produced syntactically-valid but semantically-wrong output (each case mapped to the wrong body),
// which the syntax safety net cannot catch, so we assert the case -> body mapping explicitly.
func TestDecompileSwitchCaseOrder(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/switch_case_order.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
	// pick(int): case 0 -> "zero", case 1 -> "one", case 2 -> "two", default -> "many".
	// The bug reversed this ordering, so verify the bodies appear in the correct relative order.
	order := []string{`"zero"`, `"one"`, `"two"`, `"many"`}
	prev := -1
	for _, lit := range order {
		idx := strings.Index(source, lit)
		if idx < 0 {
			t.Fatalf("expected output to contain %s\n----- source -----\n%s", lit, source)
		}
		if idx <= prev {
			t.Fatalf("switch case bodies out of order (%s appeared too early) - case mapping corrupted\n----- source -----\n%s", lit, source)
		}
		prev = idx
	}
}

// TestDecompileNoUnreachableJump guards the unreachable-code fix: a conditional return/throw
// inside a loop used to make the decompiler append a structural `break;`/`continue;` right after
// the `return`/`throw`, which the ANTLR syntax net accepts but javac rejects as an "unreachable
// statement". The decompiler must now drop those dead trailing jumps. We assert no `break;`/
// `continue;` line immediately follows a `return`/`throw` line (and the method fully decompiles).
func TestDecompileNoUnreachableJump(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/unreachable_break.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
	if strings.Contains(source, "yak-decompiler") {
		t.Fatalf("expected full decompilation, got a stub\n----- source -----\n%s", source)
	}
	lines := strings.Split(source, "\n")
	isTerminal := func(s string) bool {
		s = strings.TrimSpace(s)
		return strings.HasPrefix(s, "return ") || s == "return;" ||
			strings.HasPrefix(s, "throw ")
	}
	isDeadJump := func(s string) bool {
		s = strings.TrimSpace(s)
		return s == "break;" || s == "continue;" ||
			strings.HasPrefix(s, "break ") || strings.HasPrefix(s, "continue ")
	}
	for i := 1; i < len(lines); i++ {
		if isDeadJump(lines[i]) && isTerminal(lines[i-1]) {
			t.Fatalf("unreachable jump %q after terminal %q (line %d)\n----- source -----\n%s",
				strings.TrimSpace(lines[i]), strings.TrimSpace(lines[i-1]), i+1, source)
		}
	}
}

// TestDecompileLoopControlFlow guards the loop control-flow correctness fixes. Three distinct
// bytecode bugs silently corrupted loop semantics while still parsing through the syntax net, so we
// assert the structural signatures explicitly (a javac-free guard; the executable end-to-end check
// lives in TestLoopSemanticsRoundTrip):
//   - descending loop: `iinc i,-1` was read unsigned and always rendered `i++`, turning `for(;;i--)`
//     into an infinite/ascending loop. The descending body must now render `var2--`.
//   - bottom-tested do-while: RebuildLoopNode appended the redirected back-edge, swapping the
//     condition node's two successors so break/continue landed on the wrong branch. The exit
//     condition must now guard `break`, not `continue`.
//   - nested loop: a nested loop whose exit flows straight to the outer `continue` left the inner
//     do-while(true) with no break and a dangling, unreachable `continue;`/`break;` after it that
//     javac rejects. No dead jump may follow a `} while (true);`.
func TestDecompileLoopControlFlow(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/loop_control_flow.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
	if strings.Contains(source, "yak-decompiler") {
		t.Fatalf("expected full decompilation, got a stub\n----- source -----\n%s", source)
	}

	// Whitespace-insensitive view so the assertions do not depend on indentation/formatting.
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(source)

	// descending loop: iinc -1 must reconstruct as a decrement, not the old `++`.
	if !strings.Contains(compact, "var2--") {
		t.Fatalf("descending loop did not reconstruct a decrement (iinc-sign regression)\n----- source -----\n%s", source)
	}

	// bottom-tested do-while: the exit condition must guard break (not continue).
	if !strings.Contains(compact, ">=(var0)){break;") {
		t.Fatalf("do-while exit branch missing break (back-edge polarity regression)\n----- source -----\n%s", source)
	}
	if strings.Contains(compact, ">=(var0)){continue;") {
		t.Fatalf("do-while inverted: exit condition guards continue instead of break\n----- source -----\n%s", source)
	}

	// nested loop: no unreachable jump may follow an infinite do-while.
	lines := strings.Split(source, "\n")
	for i := 1; i < len(lines); i++ {
		cur := strings.TrimSpace(lines[i])
		prev := strings.TrimSpace(lines[i-1])
		isDeadJump := cur == "break;" || cur == "continue;" ||
			strings.HasPrefix(cur, "break ") || strings.HasPrefix(cur, "continue ")
		if isDeadJump && strings.HasSuffix(prev, "while (true);") {
			t.Fatalf("unreachable %q after infinite loop close %q (line %d)\n----- source -----\n%s",
				cur, prev, i+1, source)
		}
	}
}

// TestDecompileNegativeLiterals guards the bipush/sipush sign fix: the bipush operand is a signed
// byte and the sipush operand a signed short, but they were read unsigned, so -5 (0xFB) decompiled
// as 251 and -3000 (0xF448) as 62536. This is silent corruption the syntax net cannot catch (the
// wrong number still parses), so we assert the negative literals are reconstructed faithfully.
func TestDecompileNegativeLiterals(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/negative_literals.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
	if strings.Contains(source, "yak-decompiler") {
		t.Fatalf("expected full decompilation, got a stub\n----- source -----\n%s", source)
	}
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(source)
	for _, want := range []string{"return-5;", "return-3000;", "return100;", "return3000;"} {
		if !strings.Contains(compact, want) {
			t.Fatalf("expected decompiled output to contain %q (bipush/sipush sign regression)\n----- source -----\n%s", want, source)
		}
	}
	for _, bad := range []string{"return251;", "return62536;"} {
		if strings.Contains(compact, bad) {
			t.Fatalf("found unsigned-read literal %q - bipush/sipush sign regression\n----- source -----\n%s", bad, source)
		}
	}
}

//go:embed testdata/regression/*.class
var regressionFS embed.FS

// TestDecompileSyntaxRegression decompiles a set of real-world .class files that previously
// produced syntactically-invalid Java, and asserts the decompiled output now parses cleanly
// through the java2ssa frontend. Each entry guards a specific decompiler fix.
func TestDecompileSyntaxRegression(t *testing.T) {
	cases := []struct {
		file string
		desc string
		// substrings that MUST appear in the decompiled output (positive assertions)
		mustContain []string
		// substrings that MUST NOT appear (the previously-buggy rendering)
		mustNotContain []string
	}{
		{
			file: "lambda_methodref.class",
			desc: "invokedynamic lambda metafactory: method references and lambda expressions",
			// constructor reference must use ::new, static method reference must be Class::method
			mustContain: []string{"::new", "GeoTileGridAggregation::setupGeoTileGridAggregationDeserializer"},
			// the impl method must not be inlined as a full method declaration in argument position
			mustNotContain: []string{"::<init>", "ObjectBuilderDeserializer.lazy(GeoTileGridAggregation$Builder::new,protected"},
		},
		{
			file:           "annotation_classvalue.class",
			desc:           "annotation class-valued element renders as Type.class",
			mustContain:    []string{".class"},
			mustNotContain: []string{"LaQute/bnd/signing/JartoolSigner$Config;"},
		},
		{
			file:           "enum_subclass.class",
			desc:           "synthetic enum-constant subclass rendered as a normal class",
			mustNotContain: []string{"enum FileMagicNumber$"},
		},
		{
			file:           "string_escape_esc.class",
			desc:           "ESC control char escaped as \\u001b instead of \\x1b",
			mustNotContain: []string{`\x1b`},
		},
		{
			file:           "string_escape_cesu8.class",
			desc:           "CESU-8 / invalid bytes escaped as \\u00XX instead of \\xXX",
			mustNotContain: []string{`\xed`, `\xa1`},
		},
		{
			file:           "string_escape_vtab.class",
			desc:           "vertical tab escaped as \\u000b instead of \\v",
			mustNotContain: []string{`\v`},
		},
		{
			file:           "array_classliteral.class",
			desc:           "array class literal rendered as T[].class",
			mustContain:    []string{"[].class"},
		},
		{
			file:           "catch_primitive_type.class",
			desc:           "catch clause with degraded primitive type falls back to Throwable",
			mustNotContain: []string{"catch(boolean", "catch(int", "catch(long", "catch(double"},
		},
		{
			file: "decompile_stub_partial.class",
			desc: "post-increment side effect inside a ternary folds into `x++` and the method decompiles fully",
			// hasNext() and next() both decompile; next() is `(cond) ? (this.index++) : (... - this.index++)`
			mustContain:    []string{"boolean hasNext()", "int next()", "this.index++"},
			mustNotContain: []string{"yak-decompiler", "throw new RuntimeException"},
		},
		{
			file: "empty_slot_stub.class",
			desc: "incomplete stack simulation leaking 'empty slot value' degrades the method to a stub instead of emitting invalid code",
			// the offending method is stubbed; the internal placeholder never reaches the output
			mustContain:    []string{"yak-decompiler"},
			mustNotContain: []string{"empty slot value"},
		},
		{
			file:           "module_info.class",
			desc:           "module-info synthetic descriptor renders as a valid compilation unit, not `class module-info {}`",
			mustNotContain: []string{"class module-info"},
		},
		{
			file:           "float_double_consts.class",
			desc:           "float/double constant fields render as valid Java literals with F/D suffix",
			mustContain:    []string{"3.14F", "2.718281828D", "-1.5F"},
		},
		{
			file: "ternary_in_try.class",
			desc: "value-producing ternary inside a try: structuring fails, so the method degrades to an honest stub instead of leaking `X = Exception;` + a bogus catch-all-rethrow",
			// the corrupted renderings must never reach output; the broken method is stubbed,
			// while the sibling ternary (no try) and plain try/catch still decompile fully
			mustContain:    []string{"yak-decompiler", "(var1) ? (var2) : (var3)", "catch(Exception var3)"},
			mustNotContain: []string{"= Exception;", "catch(Exception e) { throw e; }"},
		},
		{
			file: "multi_catch.class",
			desc: "multi-catch (A | B): exception-table entries sharing one handler PC reconstruct the full union clause",
			// both the 2-type and 3-type unions must be reconstructed in handler order
			mustContain: []string{
				"catch(NumberFormatException | NullPointerException",
				"catch(IllegalStateException | IllegalArgumentException | NullPointerException",
				"catch(ArithmeticException",
			},
			// a single catch must not gain a spurious union separator, and nothing is stubbed
			mustNotContain: []string{"catch(ArithmeticException |", "yak-decompiler"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := regressionFS.ReadFile("testdata/regression/" + tc.file)
			if err != nil {
				t.Fatalf("read embedded class %s failed: %v", tc.file, err)
			}
			source, err := javaclassparser.Decompile(raw)
			if err != nil {
				t.Fatalf("decompile %s failed (%s): %v", tc.file, tc.desc, err)
			}
			// the decompiled output must parse as syntactically-valid Java
			if _, ferr := java2ssa.Frontend(source); ferr != nil {
				t.Fatalf("frontend parse failed for %s (%s): %v\n----- decompiled source -----\n%s", tc.file, tc.desc, ferr, source)
			}
			for _, must := range tc.mustContain {
				if !strings.Contains(source, must) {
					t.Errorf("%s (%s): expected output to contain %q\n----- decompiled source -----\n%s", tc.file, tc.desc, must, source)
				}
			}
			for _, mustNot := range tc.mustNotContain {
				if strings.Contains(source, mustNot) {
					t.Errorf("%s (%s): expected output NOT to contain %q\n----- decompiled source -----\n%s", tc.file, tc.desc, mustNot, source)
				}
			}
		})
	}
}

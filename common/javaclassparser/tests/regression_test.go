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

// TestDecompileTypeAnnotations guards the JSR 308 type-annotation (RuntimeVisibleTypeAnnotations)
// parsing fix. That attribute embeds type_annotation entries, each prefixed by target_type +
// target_info + type_path before the regular annotation body, so it cannot be read with the plain
// RuntimeVisibleAnnotations reader. The old code reused that reader via struct embedding, consumed
// the wrong byte count, and desynced the class reader, corrupting every later attribute-name index
// into "parse class error: get utf8 error: Invalid constant pool index!" - a hard whole-class
// failure. This Spring class (a @ConfigurationProperties bean with a javax.validation @NotEmpty
// type-use annotation) reproduced it; we now skip the attribute byte-exactly and must fully decompile.
func TestDecompileTypeAnnotations(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/type_annotations.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed (RuntimeVisibleTypeAnnotations parse regression): %v", err)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
	if strings.Contains(source, "yak-decompiler") {
		t.Fatalf("expected full decompilation, got a stub\n----- source -----\n%s", source)
	}
}

// TestDecompileCyclicStatementTreeNoCrash locks in the stack-overflow hardening. On certain real
// classes the structuring stage emitted a self-referential container (an IfStatement whose own body
// contained itself). Every recursive tree walker (RewriteVar, Statement.ReplaceVar, Statement.String,
// ...) then recursed without bound, which surfaces as Go's UNRECOVERABLE `fatal error: stack overflow`
// and crashed the whole host process - the per-method recover nets cannot catch a fatal error. The
// decompiler now detects the cyclic/pathological tree iteratively (AssertStatementsAcyclic) and raises
// an ordinary, recoverable panic so the affected method degrades to a clean stub. This zxing Aztec
// Encoder reproduced the crash via Encoder.encode. The mere fact that this test process survives and
// returns proves no fatal stack overflow occurred; we additionally require a syntactically-valid result.
func TestDecompileCyclicStatementTreeNoCrash(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/cyclic_if_tree.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, derr := javaclassparser.Decompile(raw)
	if derr != nil {
		t.Fatalf("decompile returned error (should degrade gracefully, not error): %v", derr)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
}

// TestDecompileEmptyCatchPop guards the empty/pop catch-handler fix. An empty `catch` whose unused
// exception is discarded with `pop` (the ECJ idiom, also produced by older javac) has no leading
// exception-store assignment, so the body-content heuristic alone mis-classified the handler as the
// try body and produced a "try with no catch" — a malformed try that degraded the whole method to a
// stub. TryRewriter now classifies handlers by the structural IsCatchStart marker captured from the
// exception table and synthesizes the catch variable, so this commons-logging SimpleLog (whose
// <clinit> and getStringProperty use empty catches) must decompile with no stub and parse cleanly.
func TestDecompileEmptyCatchPop(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/empty_catch_pop.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, derr := javaclassparser.Decompile(raw)
	if derr != nil {
		t.Fatalf("decompile failed: %v", derr)
	}
	if strings.Contains(source, "yak-decompiler") {
		t.Fatalf("expected full decompilation (empty-catch handler), got a stub\n----- source -----\n%s", source)
	}
	if !strings.Contains(source, "catch(") {
		t.Fatalf("expected reconstructed catch clauses, got none\n----- source -----\n%s", source)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
}

// TestDecompileResetTypeNilNoPanic guards the SlotValue.ResetValue nil-type fix. Under incomplete
// stack simulation a slot's temp type can be nil; ResetTypeRef dereferenced it and panicked the
// method into a stub. This jhlabs ContourCompositeContext (its compose() over Rasters) reproduced
// the nil dereference; it must now decompile fully without any stub.
func TestDecompileResetTypeNilNoPanic(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/reset_type_nil.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, derr := javaclassparser.Decompile(raw)
	if derr != nil {
		t.Fatalf("decompile failed: %v", derr)
	}
	if strings.Contains(source, "yak-decompiler") {
		t.Fatalf("expected full decompilation (nil temp-type slot), got a stub\n----- source -----\n%s", source)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
}

// TestDecompileSwitchSharedTargetNoPanic guards the switch shared-target fix. When several case
// values share one handler, the parse-time case-to-index map recorded len-1 (the last appended
// target) instead of the existing target's real index; once later passes shrank node.Next that
// stale index ran past the successor slice and the switch rewriter panicked with index-out-of-range,
// degrading the method to a stub. This flexmark CoreNodeFormatter (a large node-dispatch switch)
// reproduced it; it must now decompile fully without any stub.
func TestDecompileSwitchSharedTargetNoPanic(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/switch_shared_target.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, derr := javaclassparser.Decompile(raw)
	if derr != nil {
		t.Fatalf("decompile failed: %v", derr)
	}
	if strings.Contains(source, "yak-decompiler") {
		t.Fatalf("expected full decompilation (switch shared targets), got a stub\n----- source -----\n%s", source)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
}

// TestDecompileParamCopyFold guards the GetRealValue parameter-placeholder fix. Method parameters
// are seeded with an empty-string placeholder value. The `aload; astore; aload; putfield` idiom
// (`var2 = param; this.field = var2;`) folds the single-use temp var2 by inlining its real value;
// GetRealValue unwrapped the temp through the parameter ref into that empty placeholder, so the
// inlined right-hand side rendered empty: `this.value = ;` (invalid Java -> the constructor degraded
// to a stub). This jsqlparser HexValue (constructor `this.value = value;` compiled via the temp copy)
// reproduced it; it must now decompile fully with the assignment intact and parse cleanly.
func TestDecompileParamCopyFold(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/param_copy_fold.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, derr := javaclassparser.Decompile(raw)
	if derr != nil {
		t.Fatalf("decompile failed: %v", derr)
	}
	if strings.Contains(source, "yak-decompiler") {
		t.Fatalf("expected full decompilation (param-copy fold), got a stub\n----- source -----\n%s", source)
	}
	if !strings.Contains(source, "this.value = var") {
		t.Fatalf("expected `this.value = var...` assignment, got empty RHS\n----- source -----\n%s", source)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
}

// TestDecompileTernaryValueStoreFold guards the dead-end store collapse. A value-ternary whose
// condition is a compound short-circuit and whose result is stored to a single-use local that is then
// consumed (`local = (a || b) ? X : Y; return use(local)`) left the now-dead store node dangling on a
// fork (entry -> {dead store, consumer}) after the local was inlined into the consumer, so the entry
// became a non-condition node with two successors and the method aborted with "multiple next". The
// method must now decompile fully with the full short-circuit ternary preserved and parse cleanly.
func TestDecompileTernaryValueStoreFold(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/ternary_value_store_fold.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, derr := javaclassparser.Decompile(raw)
	if derr != nil {
		t.Fatalf("decompile failed: %v", derr)
	}
	if strings.Contains(source, "yak-decompiler") {
		t.Fatalf("expected full decompilation (ternary value store fold), got a stub\n----- source -----\n%s", source)
	}
	if !strings.Contains(source, "isEmpty()") || !strings.Contains(source, "?") {
		t.Fatalf("expected the full short-circuit ternary preserved, got:\n----- source -----\n%s", source)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
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

// TestDecompileBalancedTernary guards the structural ternary reconstruction. A conditional whose
// BOTH arms are themselves ternaries (a balanced tree c?(a?:):(b?:)) defeated the old chain-based
// combiner: the bottom-up merge detection only recorded the if-nodes nearest the leaves, so the
// outer condition (which has no direct leaf arm) was missing and an arm was silently dropped,
// producing an empty-slot stub or a malformed if. The reconstruction now rebuilds the if-node tree by
// structure and adopts dominating conditions, so these fully decompile. We assert no stub and that
// each conditional is reconstructed with both nested arms intact.
func TestDecompileBalancedTernary(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/balanced_ternary.class")
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
		t.Fatalf("balanced ternary degraded to a stub (structural reconstruction regression)\n----- source -----\n%s", source)
	}
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(source)
	// balanced(x,y) must keep a nested ternary in BOTH arms; the old combiner collapsed one arm.
	if !strings.Contains(compact, "?(((var1)>(5))?(1):(2)):(((var1)>(5))?(3):(4))") {
		t.Fatalf("balanced both-arms ternary not reconstructed (an arm was dropped)\n----- source -----\n%s", source)
	}
	// boolArms must reconstruct boolean comparison arms (type propagated), not int 1/0.
	if !strings.Contains(compact, "?((var0)<(10)):((var0)<(-10))") {
		t.Fatalf("boolean-arm ternary not reconstructed\n----- source -----\n%s", source)
	}
}

// TestDecompileFastjsonJSONWriterSetPathCrossChecked locks in the fastjson2 JSONWriter.setPath
// behavior that was cross-checked against javap plus CFR/Vineflower. The regression was not just
// whether the class fully decompiled: setPath(int,Object) must preserve the `dup_x1/putfield` child
// path assignments inside the value ternary, and all setPath overloads must hoist the shared
// `previous` local declaration so branch assignments are visible at the final `return previous`.
func TestDecompileFastjsonJSONWriterSetPathCrossChecked(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/fastjson2_jsonwriter_setpath.class")
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
		t.Fatalf("JSONWriter degraded to a stub\n----- source -----\n%s", source)
	}
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(source)
	mustContain := []string{
		"this.path=((var1)==(0))?",
		"this.path.child0=newJSONWriter$Path(this.path,var1)",
		"this.path.child1=newJSONWriter$Path(this.path,var1)",
		"JSONWriter$Pathvar3;if((var2)==(this.rootObject)){var3=JSONWriter$Path.ROOT;",
		"this.refs.put(var2,this.path);returnnull;",
		"returnvar3.toString();",
	}
	for _, want := range mustContain {
		if !strings.Contains(compact, want) {
			t.Fatalf("expected cross-checked JSONWriter setPath fragment %q\n----- source -----\n%s", want, source)
		}
	}
	if strings.Contains(compact, "JSONWriter$Pathvar3_1") {
		t.Fatalf("setPath previous local was split across branches (var3_1 leaked)\n----- source -----\n%s", source)
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
			file: "ternary_return_split.class",
			desc: "value-ternary return whose false arm computes its value through local stores " +
				"(ECJ pre-sized StringBuilder); the shared return is tail-duplicated into a real if/else " +
				"instead of stubbing with 'multiple next'. Guava CaseFormat.firstCharOnlyToUpper.",
			// the empty-string guard becomes a real if returning the input, and the arm stores survive
			mustContain: []string{"firstCharOnlyToUpper", "isEmpty()", "Ascii.toUpperCase", "Ascii.toLowerCase"},
			// must fully decompile (no stub) and not leak a bare ternary-condition fork
			mustNotContain: []string{"yak-decompiler"},
		},
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
			file:        "array_classliteral.class",
			desc:        "array class literal rendered as T[].class",
			mustContain: []string{"[].class"},
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
			desc: "short-circuit boolean predicates whose conditions converge on shared iconst_0/iconst_1 leaves (a middle condition the legacy combiner could not callback) are rebuilt as full &&/|| expressions by the principled merge-value tree builder, instead of leaking an empty stack slot and degrading to a stub",
			// the previously-stubbed predicates now reconstruct as idiomatic boolean expressions
			mustContain: []string{
				"((var0) >= (48)) && ((var0) <= (57))",
				"((var0) <= (whitespaceFlags.length)) && (whitespaceFlags[var0])) || ((var0) == (12288))",
			},
			// no stub marker and no internal placeholder reach the output anymore
			mustNotContain: []string{"yak-decompiler", "empty slot value"},
		},
		{
			file:           "module_info.class",
			desc:           "module-info synthetic descriptor renders as a valid compilation unit, not `class module-info {}`",
			mustNotContain: []string{"class module-info"},
		},
		{
			file:        "float_double_consts.class",
			desc:        "float/double constant fields render as valid Java literals with F/D suffix",
			mustContain: []string{"3.14F", "2.718281828D", "-1.5F"},
		},
		{
			file: "ternary_in_try.class",
			desc: "value-producing ternary inside a try fully reconstructs: the try node's catch handler is identified by its caught-exception store (not by successor position), so a reordered successor list no longer drops the catch and leaves a malformed try",
			// the ternary-in-try, the sibling ternary (no try), and the plain try/catch all
			// decompile fully; the old corrupted/stubbed renderings must never reach output
			mustContain:    []string{"return (var1) ? (var2) : (var3);", "catch(Exception var4)", "return -1;"},
			mustNotContain: []string{"yak-decompiler", "= Exception;", "catch(Exception e) { throw e; }"},
		},
		{
			file: "generic_field_type.class",
			desc: "a generic field whose type argument is FQN-disambiguated (Map<Date, java.sql.Date>, the second Date kept fully-qualified) renders as a valid field instead of being corrupted into a bogus `import Set<...>;` + a `Date>` type and dropped: the field type string must not be re-fed through Import/ShortTypeName",
			// the field and both its type-arg imports survive; the rendered type is well-formed
			mustContain:    []string{"Map<Date, java.sql.Date> dateMap;", "import java.util.Map;", "import java.util.Date;"},
			mustNotContain: []string{"import Map<", "import Set<", "yak-decompiler"},
		},
		{
			file: "merge_condition_in_try.class",
			desc: "a try body that begins with a merge-condition (an if whose head is also a control-flow " +
				"join, dominated by a pre-try join rather than the try marker) is structured into a real " +
				"if/else before the try is built, instead of leaking a bare `if(cond);` (Family B). " +
				"Mirrors Hutool ImgUtil.write / Ant Exec.",
			// the in-try condition reconstructs as a real if/else and the method does not stub
			mustContain:    []string{"try{", "if ((var3) != (null))", "sink(var3)", "sink(var1)"},
			mustNotContain: []string{"yak-decompiler", "if (var3) != (null);"},
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
		{
			file: "jackson_from_string_deserializer_multicatch.class",
			desc: "a multi-catch exception value stored into a normal local after the catch must not render " +
				"the ordinary local declaration as `A | B x = null`; the union type is legal only in the catch parameter.",
			mustContain: []string{
				"Exception var4 = null;",
				"catch(IllegalArgumentException | MalformedURLException",
				"withCause(var4)",
			},
			mustNotContain: []string{
				"IllegalArgumentException | MalformedURLException var4 = null",
				"yak-decompiler",
				"post-decompile syntax",
			},
		},
		{
			file: "jackson_cbor_empty_enum_feature.class",
			desc: "an enum with no constants but normal fields/methods (CBORParser.Feature) needs the empty " +
				"enum body separator before declarations, and member validation must wrap enum members with that separator.",
			mustContain: []string{
				"public enum CBORParser$Feature implements FormatFeature",
				";\n\tfinal boolean _defaultState;",
				"public static int collectDefaults()",
				"public boolean enabledByDefault()",
				"public int getMask()",
				"public boolean enabledIn(int var1)",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"post-decompile syntax",
			},
		},
		{
			file: "xom_unicodeutil_load_compositions.class",
			desc: "a nested no-catch try marker inside the XOM composition loading loop should flatten " +
				"when the body is safe; the surrounding IOException/Throwable handlers preserve the bytecode semantics.",
			mustContain: []string{
				"private static void loadCompositions(ClassLoader var0)",
				"compositions.put(var1.readUTF(),var1.readUTF())",
				"catch(IOException var2)",
				"catch(Throwable var2)",
				"throw var2;",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"try without catch handler",
				"catch(Exception e) { throw e;",
			},
		},
		{
			file: "jakarta_mail_protocol_retr.class",
			desc: "jakarta.mail Protocol.retr(int,int) contains multi-exit conditions around LIST/RETR " +
				"response parsing; residual condition nodes with extra exits should not be rejected by the final checker.",
			mustContain: []string{
				"batchCommandStart",
				"batchCommandContinue",
				"multilineCommandStart",
				"readMultilineResponse",
				"catch(RuntimeException",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"if statement must have",
				"try without catch handler",
			},
		},
		{
			file: "multiple_next_early_return.class",
			desc: "an if whose arm can early-return while the other arm falls through to a shared continuation must not abort structuring with 'multiple next'",
			// deserializeAndSet must fully reconstruct (no stub); the early-return arm and the
			// fall-through try/catch both survive into the output (Jackson PropertyDeserializer).
			mustContain:    []string{"deserializeAndSet", "_skipNulls"},
			mustNotContain: []string{"yak-decompiler", "multiple next"},
		},
		{
			file: "assert_guard_cyclic.class",
			desc: "a method with several `assert`s whose shared/overlapping throw targets make the value-merge structuring leave an orphaned $assertionsDisabled guard + its `throw new AssertionError()` (rendering the fatal `if (cond);`) must fold the throw into a real if-body instead of stubbing the whole method",
			// backport ArrayDeque.checkInvariants fully reconstructs: the AssertionError throws survive
			// and no method is stubbed. The corruption marker must never reach output.
			mustContain:    []string{"checkInvariants", "AssertionError"},
			mustNotContain: []string{"yak-decompiler", "post-decompile syntax"},
		},
		{
			file: "groovy_constructor_switch.class",
			desc: "a Groovy selectConstructorAndTransformArguments switch (logback gaffer NestingType.$INIT) " +
				"threads a freshly-allocated object across switch arms via dup_x1/dup2_x1 on top of an " +
				"operand stack of depth 2. The fix rebuilds each case/default body's operand stack from " +
				"the switch instruction's post-selector StackEntry instead of a shared mutable variable " +
				"that earlier arms (an athrow/return ending with an empty stack) clobbered, which left " +
				"later case bodies underflowing and leaking empty-slot placeholders.",
			// $INIT fully reconstructs: a switch over selectConstructorAndTransformArguments whose
			// arms build new NestingType(...) constructor calls
			mustContain: []string{"selectConstructorAndTransformArguments", "new NestingType(", "$INIT"},
			// no stub, no leaked internal placeholder
			mustNotContain: []string{"yak-decompiler", "empty slot value"},
		},
		{
			file: "druid_tddlhint_shared_container.class",
			desc: "a for(;;) parser loop with break/continue in nested else-if arms, followed by post-loop " +
				"code (druid TDDLHint.<init>'s `if (functions.size() > 0) type = Function`). The structuring " +
				"produced a shared container (the post-loop IfStatement appeared in both the switch/loop-tail " +
				"level AND inside a do-while body). AssertStatementsAcyclic previously panicked on any shared " +
				"container; now it distinguishes true cycles (a node is its own ancestor → infinite recursion, " +
				"must panic) from shared DAG nodes (two independent parents → finite, safe to skip the duplicate).",
			mustContain:    []string{"TDDLHint", "functions.size"},
			mustNotContain: []string{"yak-decompiler", "cyclic"},
		},
		{
			file: "mybatis_plus_abstract_kt_wrapper.class",
			desc: "Kotlin vararg mapNotNull loop in MyBatis-Plus AbstractKtWrapper.columnsToString has a " +
				"nop between two checkcast instructions after the loop. Removing nop must preserve the successor " +
				"source stack, and the do-while(true) break guard must remain semantically aligned with CFR/Vineflower.",
			mustContain: []string{
				"columnsToString(boolean",
				"CollectionsKt.joinToString$default",
				"var6.add(var17)",
				"if ((var10) < (var11))",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"empty slot value",
			},
		},
		{
			file: "transmittable_threadlocal_ctbehavior.class",
			desc: "Javassist CtBehavior.insertAfter contains nested do-while/if break guards. The do-while " +
				"break-guard normalization must stay local to a direct break body and never consume an outer if body.",
			mustContain: []string{
				"insertAfter(String var1, boolean var2, boolean var3)",
				"var7.hasNext()",
				"insertAfterAdvice",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"if (!(var7.hasNext()){",
				") >= (var14)))",
			},
		},
		{
			file: "dmjdbc_keyword_field.class",
			desc: "Obfuscated DmJdbc class contains a field literally named `do`; field declarations and " +
				"member accesses must be rendered with a safe Java identifier instead of dropping the field and stubbing methods.",
			mustContain: []string{
				"private short do_;",
				"this.do_ = var3",
				"setShort(20,this.do_)",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"this.do;",
				" short do;",
			},
		},
		{
			file: "mybatis_plus_ktupdatewrapper_lambda_name.class",
			desc: "Kotlin synthetic lambda method names may contain JVM-only characters such as '-' " +
				"(set$lambda-0). Method declarations and method references must be rendered with a safe Java identifier instead of dropping the method.",
			mustContain: []string{
				"var2::set$lambda_0",
				"private static final void set$lambda_0",
				"formatParam(var1,var2)",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"set$lambda-0",
			},
		},
		{
			file: "pagehelper_sqlparser_default_lambda.class",
			desc: "Interface static final field DEFAULT is initialized from <clinit> with a no-capture " +
				"lambda. The assignment must be hoisted into the field initializer because Java source interfaces cannot contain static initializer blocks.",
			mustContain: []string{
				"public static final SqlParser DEFAULT = (String l0) ->",
				"CCJSqlParser var1 = CCJSqlParserUtil.newParser(l0)",
				"return var1.Statement()",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"static  {",
				"public static final SqlParser DEFAULT;",
			},
		},
		{
			file: "zxing_encoder_cyclic_container.class",
			desc: "ZXing Aztec Encoder.encode produced a self-referential IfStatement after CFG structuring. " +
				"The impossible container backlink must be pruned before recursive passes so the method is not stubbed.",
			mustContain: []string{
				"public static AztecCode encode(byte[] var0, int var1, int var2)",
				"new HighLevelEncoder(var0).encode()",
				"drawModeMessage(",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"cyclic container statement",
			},
		},
		{
			file: "hazelcast_annotation_parameter_value_compareto.class",
			desc: "Hazelcast shaded ClassGraph AnnotationParameterValue.compareTo has an outer value ternary " +
				"whose condition was lost after nested null-check ternaries, leaking an empty stack slot. The renderer should recover the enclosing guard and keep the method.",
			mustContain: []string{
				"public int compareTo(AnnotationParameterValue var1)",
				"Object var3 = this.getValue()",
				"Object var4 = var1.getValue()",
				"toStringParamValueOnly().compareTo",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"empty slot value",
			},
		},
		{
			file: "hazelcast_record_assertion_clinit.class",
			desc: "Hazelcast Record is an interface whose <clinit> only leaves an assertion-disabled no-op guard after static field initializer hoisting. " +
				"The no-op guard should be skipped instead of logging an un-representable static initializer warning.",
			mustContain: []string{
				"public interface Record",
				"public static final long EPOCH_TIME = TimeUtil.zeroOutMs(1514764800000L)",
				"public static final Object NOT_CACHED = new Object()",
			},
			mustNotContain: []string{
				"static  {",
				"yak-decompiler",
			},
		},
		{
			file: "hazelcast_row_assertion_clinit.class",
			desc: "Hazelcast Row is an interface with an assertion-only <clinit>. The initializer is a source-level no-op and should be skipped.",
			mustContain: []string{
				"public interface Row extends RowBatch",
				"public default Row getRow(int var1)",
			},
			mustNotContain: []string{
				"static  {",
				"yak-decompiler",
			},
		},
		{
			file: "icu4j_collation_data_wide_iinc.class",
			desc: "ICU4J CollationData.getScriptIndex branches to a wide iinc instruction. " +
				"The WIDE prefix offset must be used for jump target mapping; otherwise the branch is miswired to OP_START and assertion folding sees nil refs.",
			mustContain: []string{
				"private int getScriptIndex(int var1)",
				"if ((var1) < (0))",
				"return this.scriptsIndex[var1]",
				"var1 = (var1) - (4096)",
				"return this.scriptsIndex[(this.numScripts) + (var1)]",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"nil pointer dereference",
				"do{\n\n\t\t} while (true)",
			},
		},
		{
			file: "okhttp_framed_connection_varargs_ctor.class",
			desc: "OkHttp anonymous NamedRunnable constructor is flagged varargs, but its final descriptor parameter is ErrorCode rather than an array. " +
				"The dumper must only render varargs syntax when the final parameter type is actually an array.",
			mustContain: []string{
				"class FramedConnection$1 extends NamedRunnable",
				"FramedConnection$1(FramedConnection var1, String var2, Object[] var3, int var4, ErrorCode var5)",
				"super(var2,var3)",
				"this.val$errorCode = var5",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"nil pointer dereference",
				"ErrorCode...",
			},
		},
		{
			file: "okhttp_headers_companion_map_of.class",
			desc: "OkHttp Kotlin Headers.Companion.of(Map) has a null-check throw arm that leaves a duplicated value on a terminal athrow path. " +
				"Mismatched candidate if-merge stack sizes must fall back to ordinary merge handling instead of stubbing the method.",
			mustContain: []string{
				"public final Headers of(Map<String, String> var1)",
				"Iterator var6 = var4.entrySet().iterator()",
				"String var10 = ((String)(var8.getKey()))",
				"return new Headers(var2,(DefaultConstructorMarker)(null))",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"invalid stack size",
				"undecompilable method body",
			},
		},
		{
			file: "okhttp_real_connection_pool_idle_count.class",
			desc: "OkHttp Kotlin RealConnectionPool.idleConnectionCount has a return merge fed by a fast-path return value and a loop result. " +
				"The ternary merge condition must be seeded from the consumed if condition so a synthetic empty condition does not leak.",
			mustContain: []string{
				"public final int idleConnectionCount()",
				"if ((var1 instanceof Collection) && (((Collection)(var1)).isEmpty()))",
				"var5.getCalls().isEmpty()",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"empty slot value",
				"undecompilable method body",
			},
		},
		{
			file: "okio_deprecated_utf8_safe_class_name.class",
			desc: "Kotlin file facade/object classes may have internal simple names beginning with '-' (for example okio/-DeprecatedUtf8). " +
				"Class declarations, static member references, constructor calls, and field types must use sanitized identifiers consistently.",
			mustContain: []string{
				"public final class _DeprecatedUtf8",
				"public static final _DeprecatedUtf8 INSTANCE",
				"_DeprecatedUtf8.INSTANCE = new _DeprecatedUtf8()",
			},
			mustNotContain: []string{
				"public final class -DeprecatedUtf8",
				"new -DeprecatedUtf8",
				"yak-decompiler",
			},
		},
		{
			file: "okio_utf8kt_slot_reuse_ternary_condition.class",
			desc: "Okio's Kotlin UTF-8 helper reuses local slots across disjoint loop-exit and loop-body paths, then builds nested ternary " +
				"conditions from if_icmp* nodes. Per-opcode local scopes must not be polluted by another path, and ternary condition slots " +
				"must be seeded from the consumed comparison operands.",
			mustContain: []string{
				"public final class _Utf8Kt",
				"public static final String commonToUtf8String(byte[] var0, int var1, int var2)",
				"return new String(var3,var8,var6)",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"empty slot value",
			},
		},
		{
			file: "okio_real_buffered_source_invalid_ternary.class",
			desc: "Okio RealBufferedSource has control-flow merges that the legacy ternary combiner cannot prove as a two-leaf expression. " +
				"Those merges should be skipped instead of failing bytecode parsing or panicking during ternary type merging.",
			mustContain: []string{
				"public final class RealBufferedSource implements BufferedSource",
				"public long indexOf(byte var1, long var2, long var3)",
				"public boolean rangeEquals(long var1, ByteString var2, int var3, int var4)",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"invalid ternary expression",
				"interface conversion",
			},
		},
		{
			file: "sparsebitset_triple_long_array_dup2.class",
			desc: "SparseBitSet stores its bitmap as a three-dimensional long array. Descriptor parsing must keep [[[J as long[][][] so " +
				"laload produces a long value; otherwise dup2/lstore/lcmp treat the value as an array/reference pair and leak an empty slot.",
			mustContain: []string{
				"public class SparseBitSet implements Cloneable, Serializable",
				"protected transient long[][][] bits",
				"public int nextClearBit(int var1)",
				"public int nextSetBit(int var1)",
				"public int previousClearBit(int var1)",
				"public int previousSetBit(int var1)",
				"private void writeObject(ObjectOutputStream var1) throws IOException, InternalError",
			},
			mustNotContain: []string{
				"protected transient long[][][][] bits",
				"yak-decompiler",
				"empty slot value",
				"undecompilable method body",
			},
		},
		{
			file: "commons_collections_extended_properties_try_loop.class",
			desc: "Commons Collections ExtendedProperties.load has a property-reading loop wrapped by a finally-style catch-all region. " +
				"The try container must not retain a self edge when loop back-edges are folded into the try body.",
			mustContain: []string{
				"public class ExtendedProperties extends Hashtable",
				"public synchronized void load(InputStream var1, String var2) throws IOException",
				"ExtendedProperties$PropertiesReader var3 = null",
				"this.isInitialized = true",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"ParseBytesCode failed: has circle",
				"undecompilable method body",
			},
		},
		{
			file: "reactor_abstract_http_server_metrics_handler_checkcast_fold.class",
			desc: "Reactor Netty AbstractHttpServerMetricsHandler passes checkcast results directly into metric recording calls. " +
				"When a checkcast temp is single-use folded, its declaration left-hand side must remain a stable local name, not the cast expression.",
			mustContain: []string{
				"abstract class AbstractHttpServerMetricsHandler extends ChannelDuplexHandler",
				"public void write(ChannelHandlerContext var1, Object var2, ChannelPromise var3)",
				"public void channelRead(ChannelHandlerContext var1, Object var2)",
				"this.recordRead(var4",
				"this.recordWrite(var4",
			},
			mustNotContain: []string{
				"String ((String)",
				"yak-decompiler",
				"post-decompile syntax",
				"undecompilable method body",
			},
		},
		{
			file: "jasperreports_excel_abstract_exporter_switch_slot_hoist.class",
			desc: "JasperReports ExcelAbstractExporter.getTextAlignHolder assigns shared alignment locals inside nested switch cases. " +
				"Switch-local declarations must be hoisted recursively and internal empty-slot placeholder assignments must not leak into source.",
			mustContain: []string{
				"abstract class ExcelAbstractExporter<",
				"public static ExcelAbstractExporter$TextAlignHolder getTextAlignHolder(JRPrintText var0)",
				"HorizontalTextAlignEnum var2;",
				"VerticalTextAlignEnum var2_1;",
				"return new ExcelAbstractExporter$TextAlignHolder(var2,var2_1,var1)",
			},
			mustNotContain: []string{
				"empty slot value",
				"panic: runtime error",
				"yak-decompiler",
				"post-decompile syntax",
				"undecompilable method body",
			},
		},
		{
			file: "jtidy_tidyutils_large_boolean_ternary.class",
			desc: "JTidy TidyUtils encodes XML character tables as very large boolean condition chains. " +
				"Ternary type and boolean reduction must be memoized so rendering does not expand shared DAGs exponentially.",
			mustContain: []string{
				"public final class TidyUtils",
				"static boolean isXMLLetter(char var0)",
				"static boolean isXMLNamechar(char var0)",
				"public static boolean isCharEncodingSupported(String var0)",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"empty slot value",
				"panic: test timed out",
				"undecompilable method body",
			},
		},
		{
			file: "saxon_builtin_type_single_use_fold.class",
			desc: "Saxon BuiltInType folds a single-use temporary after class initialization guards. " +
				"The CFG edge rewrite must not mutate predecessor edge slices while iterating them.",
			mustContain: []string{
				"abstract class BuiltInType",
				"public static SchemaType getSchemaType(int var0)",
				"public static SchemaType getSchemaTypeByLocalName(String var0)",
				"lookup.get(var0)",
				"lookupByLocalName.get(var0)",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"panic: runtime error",
				"index out of range",
				"undecompilable method body",
			},
		},
		{
			file: "openrewrite_find_indent_yaml_boolean_ctor.class",
			desc: "OpenRewrite FindIndentYamlVisitor uses boolean constructor arguments and lambda boolean setters. " +
				"Boolean invocation contexts must render bytecode 0/1 values and ternaries as Java booleans.",
			mustContain: []string{
				"public class FindIndentYamlVisitor<",
				"AtomicBoolean var4 = new AtomicBoolean(true)",
				"AtomicBoolean var9 = new AtomicBoolean(false)",
				"var4.set((var4.get()) &&",
				"return Boolean.valueOf((l0) == (32))",
			},
			mustNotContain: []string{
				"new AtomicBoolean(1)",
				"new AtomicBoolean(0)",
				"? (1) : (0)",
				"yak-decompiler",
				"panic: runtime error",
				"undecompilable method body",
			},
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

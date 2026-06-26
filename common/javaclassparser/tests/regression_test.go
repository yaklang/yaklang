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

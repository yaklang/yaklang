package tests

import (
	"embed"
	"regexp"
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

// TestDecompileLoopThenTernaryReturn guards Bug N: a `while` loop whose exit edge flows directly into
// a consumed ternary return (`while(b!=0){...} return a<0 ? -a : a`, the classic gcd shape). The
// ternary condition is collapsed (its value folded into the return) by the callback-collapse pass,
// which spliced the collapsed node's successors into the predecessor with a remove-then-append
// rewiring. When the predecessor is the loop header itself, append reordered its [exit, body]
// successors to [body, exit], inverting the loop polarity: the decompiled output guarded `break` with
// `b != 0` (so the very first b becomes the divisor of `a % 0` -> ArithmeticException) instead of
// guarding `continue`. The fix uses ReplaceNextSliceKeepOrder so the splice preserves successor
// order. This is a javac-free structural guard; the executable round-trip lives in the gcd/
// loopThenTernary cases of TestLoopSemanticsRoundTrip.
func TestDecompileLoopThenTernaryReturn(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/loop_then_ternary_return.class")
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

	// The loop condition (b != 0) must guard the loop BODY (the modulo step / continue), not break.
	// The inverted (buggy) polarity rendered `if((var1)!=(0)){break;`, which is the exact corruption.
	if strings.Contains(compact, "!=(0)){break;") {
		t.Fatalf("loop polarity inverted: `b != 0` guards break instead of the loop body (Bug N regression)\n----- source -----\n%s", source)
	}
	// The modulo back-edge step must sit on the b!=0 branch (loop body), proving the body is taken
	// while b is non-zero.
	if !strings.Contains(compact, "%(var1)") {
		t.Fatalf("loop body modulo step missing; loop structure corrupted\n----- source -----\n%s", source)
	}
	// The ternary return sign must be preserved: `a < 0 ? -a : a`.
	if !strings.Contains(compact, "?(-var0):(var0)") {
		t.Fatalf("ternary return arms corrupted (expected `a<0 ? -a : a`)\n----- source -----\n%s", source)
	}
}

// TestDecompileGuardTrailingWhileReturn guards Bug H: a `while(cond){i++}` loop immediately followed
// by `return i==n`. This is the same control-flow-polarity family as Bug N (a loop whose exit edge
// flows into a consumed condition/return); before the callback-collapse order-preserving splice fix
// it inverted the loop body and exit, turning `while(p<len && pat[p]=='*') p++;` into an infinite
// self-increment that overflows the index. The repro is the Spring AntPathMatcher `matchStrings`
// tail. javac-free structural guard; the executable round-trip lives in SpringAlgorithms.matchStrings.
func TestDecompileGuardTrailingWhileReturn(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/guard_trailing_while_return.class")
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
	// The loop guard (p<len && pat[p]=='*') must guard the loop BODY (var2++ / continue), not break.
	// The inverted polarity rendered the matching branch as `break`, which the fix forbids.
	if strings.Contains(compact, "42)){break;") {
		t.Fatalf("trailing-while loop polarity inverted: match guards break instead of increment (Bug H regression)\n----- source -----\n%s", source)
	}
	// The increment must remain inside the loop body.
	if !strings.Contains(compact, "var2++") {
		t.Fatalf("loop increment missing from body; loop structure corrupted\n----- source -----\n%s", source)
	}
	// The final exit comparison must be preserved.
	if !strings.Contains(compact, "(var2)==(var1)") {
		t.Fatalf("loop exit comparison corrupted (expected `p == len`)\n----- source -----\n%s", source)
	}
}

// TestDecompileGuardIfNotThrowBody guards Bug E/F/I: an `if (!cond) throw ...;` guard followed by a
// larger sequential body. Before the polarity fix the decompiler swapped the throw arm and the body
// relative to the condition (throwing when cond was true). This is the same branch-polarity family as
// Bug N/H. The throw must stay on the `x < 0` branch and the body on the fall-through branch. The
// executable round-trip is exercised by GuavaAlgorithms/SpringAlgorithms guard ladders.
func TestDecompileGuardIfNotThrowBody(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/guard_ifnot_throw_body.class")
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
	// The throw must be guarded by the negative check (x < 0), not by its negation. We locate the
	// `throw` and verify the nearest enclosing condition mentions `< (0)` (i.e. the guard was not
	// inverted to throw on the valid x >= 0 path).
	idxThrow := strings.Index(source, "throw new IllegalArgumentException")
	if idxThrow < 0 {
		t.Fatalf("throw statement missing\n----- source -----\n%s", source)
	}
	prefix := source[:idxThrow]
	idxCond := strings.LastIndex(prefix, "if (")
	if idxCond < 0 {
		t.Fatalf("no enclosing if for throw\n----- source -----\n%s", source)
	}
	guard := source[idxCond:idxThrow]
	if !strings.Contains(guard, "< (0)") {
		t.Fatalf("guard polarity inverted: throw is not guarded by `x < 0` (Bug E/F/I regression)\n----- guard -----\n%s\n----- source -----\n%s", guard, source)
	}
}

// TestDecompileNestedLoopContinueGuard guards Bug L: an `if (x == 0L) continue;` guard inside the
// inner of two sibling nested loops over the same array. Before the polarity fix the guard inverted
// (running the divide body when the divisor was zero -> ArithmeticException). Same control-flow
// polarity family as Bug N. The zero check must guard an EMPTY (continue) branch and the divide must
// sit on the else/fall-through branch. The executable round-trip lives in MoreGuavaAlgorithms.
func TestDecompileNestedLoopContinueGuard(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/nested_loop_continue_guard.class")
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
	// The divide must NOT sit directly on the `== 0L` true-branch (that would divide by zero). The
	// inverted polarity produced `if((var0[..])==(0L)){ ...division... }`; the fix keeps the division
	// off the zero branch (empty/continue then-body, division in else).
	if regexp.MustCompile(`==\(0L\)\)\{[^}]*\)/\(`).MatchString(compact) {
		t.Fatalf("nested-loop continue guard inverted: division on the `== 0` branch (Bug L regression)\n----- source -----\n%s", source)
	}
	// The division by the guarded element must still be present (loop body preserved).
	if !strings.Contains(compact, ")/(var0[var4])") {
		t.Fatalf("loop body division missing; structure corrupted\n----- source -----\n%s", source)
	}
}

// TestDecompileArrayPostIncrementIndex guards the post-increment-in-expression bug ("arr[i++]=v"):
// javac compiles `a[i++] = v` as `iload X; iinc X; ...`, so the OLD index is still live on the
// operand stack when the iinc runs. The decompiler used to emit the standalone increment BEFORE the
// store (`i++; a[i] = v`), shifting every index by one (wrong values / out-of-bounds). The fix folds
// the iinc into a post-increment expression on the index so the store reads `a[i++] = v`. Executable
// round-trip coverage lives in PostIncrementAlgorithms; this is the javac-free CI guard.
func TestDecompileArrayPostIncrementIndex(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/array_post_increment_index.class")
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
	// The index must be post-incremented inside the array store: `var0[var1++] = 10`.
	if !strings.Contains(compact, "var0[var1++]=10") {
		t.Fatalf("array store did not fold the post-increment index (expected `var0[var1++] = 10`)\n----- source -----\n%s", source)
	}
	// The buggy form hoists the increment before the store: `var1++; var0[var1] = 10`. Forbid it.
	if strings.Contains(compact, "var1++;var0[var1]=10") {
		t.Fatalf("post-increment hoisted before the array store (arr[i++] regression)\n----- source -----\n%s", source)
	}
}

// TestDecompileConsumedCompoundAssign guards Bug J: a compound assignment whose RESULT VALUE IS
// CONSUMED (`int r = (a[i] += 3)` / `long r = (a[i] += 3L)`). javac duplicates the freshly-computed
// value with dup_x2 (int element) / dup2_x2 (long element) so it feeds BOTH the array store and the
// consumer. The decompiler used to let the consumer resolve THROUGH the materialized shared temp down
// to its defining expression, re-evaluating it AFTER the store and double-applying the operator
// (`((a[i]+3)) * 1000` instead of `r * 1000`). Two adjacent defects were fixed: var-fold now stops at
// dup-shared temps (resolveFoldValue), and OP_DUP2_X2 tracks its ref-fold callback per item (so the
// duplicated category-2 value's two consumers register and the temp is not folded away). Executable
// round-trip coverage lives in ConsumedCompoundAlgorithms; this is the javac-free CI guard.
func TestDecompileConsumedCompoundAssign(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/consumed_compound_assign.class")
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
	// The freshly computed value must be stored into a temp, the array element written from that temp,
	// and the consumer must REFERENCE the temp (var2), not recompute the RHS.
	if !strings.Contains(compact, "var0[var1]=var2") {
		t.Fatalf("compound result temp not stored into the array element\n----- source -----\n%s", source)
	}
	if !strings.Contains(compact, "(var2)*(1000)") {
		t.Fatalf("int consumer did not reference the shared temp `var2` (dup_x2 Bug J regression)\n----- source -----\n%s", source)
	}
	if !strings.Contains(compact, "(var2)*(1000L)") {
		t.Fatalf("long consumer did not reference the shared temp `var2` (dup2_x2 Bug J regression)\n----- source -----\n%s", source)
	}
	// The buggy re-evaluation hoists the RHS into the consumer: `((a[i]+3)) * 1000`. Forbid both the
	// int and the long shapes (the operator would be applied twice).
	if strings.Contains(compact, "+(3))*(1000)") {
		t.Fatalf("int consumer re-evaluated the compound RHS (dup_x2 Bug J regression)\n----- source -----\n%s", source)
	}
	if strings.Contains(compact, "+(3L))*(1000L)") {
		t.Fatalf("long consumer re-evaluated the compound RHS (dup2_x2 Bug J regression)\n----- source -----\n%s", source)
	}
}

// TestDecompileEmptyDefaultSwitchMerge guards Bug K: a switch with an EMPTY `default: break;` whose
// target coincides with the switch's natural post-switch merge point. Every matched case `break`s
// (goto) to the same node the empty default falls through to, so that node is BOTH the default's
// "start" and the real merge. The old merge detection excluded it (it is a case start), fell back to
// treating it as the default BODY, dropped every case `break` (all cases fell through), and absorbed
// the post-switch code into `default:` (Base32 decode length miscomputed). The fix promotes that
// convergence node (reached by >=2 case bodies via unconditional break) to the real merge, drops the
// empty default, and emits the post-switch code after the switch. Covers both tableswitch (dense) and
// lookupswitch (sparse). Executable round-trip lives in SwitchFallthroughAlgorithms.
func TestDecompileEmptyDefaultSwitchMerge(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/switch_empty_default_merge.class")
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
	// Each case must keep its `break`, and the post-switch code must follow the switch close `}`.
	if !strings.Contains(compact, "case2:var2++;break;") {
		t.Fatalf("tableswitch empty-default: case break dropped (Bug K regression)\n----- source -----\n%s", source)
	}
	if !strings.Contains(compact, "break;}return((var2)*(10))+(var0);") {
		t.Fatalf("tableswitch empty-default: post-switch code absorbed into default (Bug K regression)\n----- source -----\n%s", source)
	}
	if !strings.Contains(compact, "case5000000:") || !strings.Contains(compact, "break;}return((var2)*(7))+(var0);") {
		t.Fatalf("lookupswitch empty-default: structure corrupted (Bug K regression)\n----- source -----\n%s", source)
	}
	// The empty `default: break;` is equivalent to no default and must be dropped; if it survives it
	// is absorbing post-switch code (the exact bug).
	if strings.Contains(compact, "default:") {
		t.Fatalf("empty default was not dropped (still present, absorbing post-switch code)\n----- source -----\n%s", source)
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

// TestDecompilePostBranchReassignNoSelfInit guards the post-branch reassignment slot fix. When a
// local is assigned ONLY inside if/else (or switch) arms and is then RE-assigned right after the
// branch as `x = f(x)`, RewriteVar used to mint a fresh self-referencing id for that reassignment:
// the dumper renamed the colliding declaration to `x_1` and, because the right-side read shared the
// same id, emitted `int x_1 = (x_1) + ...`, which javac rejects with "variable x_1 might not have
// been initialized". This was the blocking defect behind the codec round-trip oracle (md5 / xxHash32).
// redirectPostBlockReassignments now repoints every post-block reference of the same logical variable
// onto the hoisted id and demotes the first reassignment to a plain assignment. The seed reproduces
// both shapes: a simple if/else merge (simpleMerge, xxHash32-shaped) and a nested if-else-if merge
// (nestedMerge, md5-round-shaped). This is a javac-free structural guard; the executable end-to-end
// proof lives in TestCodecSemanticsRoundTrip.
func TestDecompilePostBranchReassignNoSelfInit(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/post_branch_reassign_slot.class")
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
	// A self-referencing initializer `<type> var = (var) ...` (the same name declared and read in its
	// own initializer) is always illegal Java; assert none of that shape survives. Go's RE2 has no
	// backreferences, so capture the declared name and the first read and compare them.
	selfInit := regexp.MustCompile(`\b(?:int|long)\s+(var\d+(?:_\d+)?)\s*=\s*\((var\d+(?:_\d+)?)\)`)
	for _, m := range selfInit.FindAllStringSubmatch(source, -1) {
		if m[1] == m[2] {
			t.Fatalf("self-referencing initialization survived: %q (post-branch reassignment slot regression)\n----- source -----\n%s", m[0], source)
		}
	}
	// The branch-merged locals must be reassigned as plain assignments, not redeclared.
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(source)
	if !strings.Contains(compact, "var2=(var2)+(var1)") {
		t.Fatalf("simpleMerge post-branch reassignment not reconstructed as a plain assignment\n----- source -----\n%s", source)
	}
	if !strings.Contains(compact, "var4=((var4)+(var1))+(var0)") {
		t.Fatalf("nestedMerge post-branch reassignment not reconstructed as a plain assignment\n----- source -----\n%s", source)
	}
}

//go:embed testdata/regression/*.class
var regressionFS embed.FS

// TestNestedClassVisibilityDemotion is a stricter guard for the nested-class visibility
// demotion (root cause B, declaration side). A class whose binary name contains '$' (i.e. a
// nested/local/anonymous class) is decompiled as a standalone top-level type; emitting it with
// its original `public`/`protected` modifier yields illegal Java ("X$Y is public, should be
// declared in a file named X$Y.java"). The fix demotes such classes to package-private. This
// test asserts the declaration carries no access modifier, independent of the syntax frontend.
func TestNestedClassVisibilityDemotion(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/nested_class_visibility.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if strings.Contains(source, "public class Outer$Inner") {
		t.Errorf("nested class regression: Outer$Inner still declared public (visibility not demoted)\n----- source -----\n%s", source)
	}
	// It must still be declared (just package-private now).
	if !strings.Contains(source, "class Outer$Inner") {
		t.Errorf("nested class regression: Outer$Inner declaration missing\n----- source -----\n%s", source)
	}
}

// TestBridgeMethodSuppression is a stricter, semantics-structural guard for the covariant-return
// bridge-method fix (root cause A in the cross-comparison report). A class implementing a generic
// interface like Supplier<String> carries a compiler-synthetic ACC_BRIDGE | ACC_SYNTHETIC
// `Object get()` that delegates to the real `String get()`. Dumping both yields illegal Java
// (cannot overload by return type alone), so the decompiler must suppress the bridge method and
// emit exactly one `get()` declaration. This test counts method declarations by name to make any
// regression (a duplicated get()) fail loudly, independent of the syntax frontend.
func TestBridgeMethodSuppression(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/bridge_method_covariant.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	// Count how many times "get() {" appears as a method declaration header. A declaration line
	// looks like "String get() {" / "Object get() {"; the buggy output had both.
	declCount := strings.Count(source, "get() {")
	if declCount != 1 {
		t.Errorf("bridge method regression: expected exactly 1 get() declaration, got %d\n----- source -----\n%s", declCount, source)
	}
}

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
			file: "bridge_method_covariant.class",
			desc: "covariant-return bridge methods (ACC_BRIDGE | ACC_SYNTHETIC) are suppressed. " +
				"A class implementing Supplier<String> has a real `String get()` plus a compiler-synthetic " +
				"`Object get()` bridge; dumping both yields illegal Java (two methods differing only by " +
				"return type). The decompiled output must declare exactly one get() method.",
			mustContain:    []string{"String get()"},
			mustNotContain: []string{"Object get()", "yak-decompiler"},
		},
		{
			file: "nested_class_visibility.class",
			desc: "a nested class (binary name Outer$Inner) decompiled as a standalone top-level type " +
				"must have its public/protected visibility demoted to package-private, because Java " +
				"forbids a public type in a file not named after it ('X$Y is public, should be declared " +
				"in a file named X$Y.java'). The class body and members are unaffected.",
			mustContain:    []string{"class Outer$Inner"},
			mustNotContain: []string{"public class Outer$Inner", "protected class Outer$Inner", "yak-decompiler"},
		},
		{
			file: "ifnonnull_branch.class",
			desc: "an `ifnonnull` (jump when != null) branch renders the condition as the fall-through " +
				"condition (`== null`), consistent with IFNULL/numeric-IF. The IfBody (TrueNode) binds to " +
				"the fall-through branch (the == null arm) and ElseBody to the jump target (the != null arm). " +
				"This matches the bytecode: ifnonnull jumps to the != null path while fall-through runs the " +
				"== null body. Rendering `!= null` here would SWAP the bodies and compute wrong results " +
				"(confirmed: commons-codec Md5Crypt is byte-for-byte correct with `== null`).",
			mustContain:    []string{"== (null)"},
			mustNotContain: []string{"!= (null)", "yak-decompiler"},
		},
		{
			file: "loop_decrement_guard.class",
			desc: "a `while (i-- > 0)` loop (javac compiles the test as load-i, iinc -1, check the " +
				"OLD value) must NOT be reconstructed as `do { i--; if (i > 0) ... }`, which tests the " +
				"decremented value and runs one fewer iteration. The fold into a post-decrement test " +
				"`if ((i--) > 0)` restores the correct iteration count — critical for algorithms whose " +
				"loop count is part of the computation (MD5-crypt B64 base64 packing).",
			mustContain:    []string{"(var2--) > (0)"},
			mustNotContain: []string{"yak-decompiler"},
		},
		{
			file: "shift_byte_promotion.class",
			desc: "a shift of a byte/short/char operand must type the result as int (JLS: shift always " +
				"promotes to int/long). Before the fix a `byte << 16` was typed byte, so the local " +
				"storing it became `byte x = (b << 16) | ...` (wrong type + wrong hash in MD5-crypt " +
				"B64.b64from24bit). The combined expression must store as int.",
			mustContain:    []string{"((var0) << (16))"},
			mustNotContain: []string{"byte v = ", "yak-decompiler"},
		},
		{
			file: "byte_local_narrowing.class",
			desc: "a byte/char/short local whose initializer is an int-valued arithmetic/bitwise/shift " +
				"expression (JLS promotes byte to int) must keep its slot type and wrap the initializer in " +
				"a narrowing cast, otherwise javac rejects it ('possible lossy conversion from int to " +
				"byte'). Real-world: commons-codec PureJavaCrc32C.update (byte x = (arr[i]^crc)&255).",
			mustContain:    []string{"byte var3 = (byte)("},
			mustNotContain: []string{"yak-decompiler"},
		},
		{
			file: "char_return_narrowing.class",
			desc: "a char-returning method whose body returns int literals (bytecode stores char " +
				"literals as ints: bipush 102 == 'f') must cast them to char, otherwise javac rejects " +
				"the return ('possible lossy conversion from int to char').",
			mustContain:    []string{"return (char) "},
			mustNotContain: []string{"yak-decompiler"},
		},
		{
			file: "bool_array_initializer.class",
			desc: "a boolean[] initializer is filled by iconst_0/iconst_1, whose values carry an " +
				"int type; they must render as true/false, not 1/0, otherwise javac rejects it " +
				"('int cannot be converted to boolean').",
			mustContain:    []string{"new boolean[]{true"},
			mustNotContain: []string{"boolean[]{1", "boolean[]{0", "yak-decompiler"},
		},
		{
			file: "wildcard_class_param.class",
			desc: "generic wildcard type arguments (`Class<?>`) render as `?`, not the illegal identifier " +
				"`__`. The old path routed `?` through SafeIdentifier, which turned it into `__` because `_` " +
				"is a Java 9+ keyword and got suffixed — producing `Class<__>` that javac rejects.",
			mustContain:    []string{"Class<?>"},
			mustNotContain: []string{"<__>", "yak-decompiler"},
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
				"enum CBORParser$Feature implements FormatFeature",
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
				"VerticalTextAlignEnum var2;",
				"HorizontalTextAlignEnum var2_1;",
				"return new ExcelAbstractExporter$TextAlignHolder(var2_1,var2,var1)",
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
		{
			file: "ecj_scope_return_nil_type.class",
			desc: "Eclipse ECJ Scope.findMethod can return a value whose inferred JavaValue.Type is nil on one reconstructed path. " +
				"Return type alignment must skip nil value types instead of panicking.",
			mustContain: []string{
				"public abstract class Scope",
				"public MethodBinding findMethod(ReferenceBinding var1, char[] var2, TypeBinding[] var3, InvocationSite var4)",
				"public MethodBinding findMethod(ReferenceBinding var1, char[] var2, TypeBinding[] var3, InvocationSite var4, boolean var5)",
				"MethodVerifier var14 = this.environment().methodVerifier()",
			},
			mustNotContain: []string{
				"yak-decompiler",
				"panic: runtime error",
				"nil pointer dereference",
				"undecompilable method body",
			},
		},
		{
			file: "ecj_type_constants_interface_clinit.class",
			desc: "ECJ TypeConstants is an interface whose <clinit> initializes final fields through a temporary local. " +
				"The temporary must be hoisted into field initializers because interface static blocks are invalid Java.",
			mustContain: []string{
				"public interface TypeConstants",
				"public static final char[][][] OTHER_WRAPPER_CLOSEABLES = new char[5][][]",
				"public static final char[][] JAVA_IO_RESOURCE_FREE_CLOSEABLES = new char[][]",
				"public static final char[] PACKAGE_INFO_NAME = \"package-info\".toCharArray()",
			},
			mustNotContain: []string{
				"static  {",
				"yak-decompiler",
				"panic: runtime error",
				"undecompilable method body",
			},
		},
		{
			file: "ecj_javadoc_tag_constants_interface_clinit.class",
			desc: "ECJ JavadocTagConstants uses interface <clinit> locals for multi-dimensional char array constants. " +
				"The decompiler must emit legal field initializers rather than an invalid static block.",
			mustContain: []string{
				"public interface JavadocTagConstants",
				"public static final char[][][] BLOCK_TAGS = new char[8][][]",
				"public static final char[][][] INLINE_TAGS = new char[8][][]",
				"public static final int ALL_TAGS_LENGTH = (BLOCK_TAGS_LENGTH) + (INLINE_TAGS_LENGTH)",
			},
			mustNotContain: []string{
				"static  {",
				"yak-decompiler",
				"panic: runtime error",
				"undecompilable method body",
			},
		},
		{
			file: "xmlbeans_qnamehelper.class",
			desc: "XMLBeans QNameHelper.hexsafe reuses local slots between byte arrays, loop indexes, and catch parameters. " +
				"Catch variables whose inferred type is polluted by a reused array slot must still render as catchable types, the " +
				"loop index must index the byte array consistently, and a catch parameter that shares a slot with a hoisted local " +
				"must be split to a fresh name so it does not illegally shadow that enclosing local.",
			mustContain: []string{
				"public class QNameHelper",
				"public static String hexsafe(String var0)",
				"var4[var5]",
				"catch(UnsupportedEncodingException var5_1)",
				"catch(Throwable var4_2)",
			},
			mustNotContain: []string{
				"catch(byte[]",
				// The catch parameter must not reuse the hoisted byte-array name var4 (illegal shadowing).
				"catch(Throwable var4)",
				"int var5_1 = 0",
				"yak-decompiler",
				"post-decompile syntax",
				"undecompilable method body",
			},
		},
		{
			file: "elasticsearch_copy_on_write_hash_map_inner_node.class",
			desc: "Elasticsearch CopyOnWriteHashMap.InnerNode.put reuses generic/object slots next to primitive parameters. " +
				"Parameter rendering must keep generated names unique, and argument folding must not inline the empty parameter placeholder into method calls.",
			mustContain: []string{
				"class CopyOnWriteHashMap$InnerNode",
				"CopyOnWriteHashMap$InnerNode<K, V> put(K var1, int var2, int var3, V var3_1, MutableValueInt var4)",
				"return this.putExisting(var1,var2,var3,var7,var3_1,var4)",
				"return this.putNew(var1,var6,var7,var3_1)",
			},
			mustNotContain: []string{
				"int var3, V var3, MutableValueInt",
				",,",
				"yak-decompiler",
				"post-decompile syntax",
				"undecompilable method body",
			},
		},
		{
			file: "elasticsearch_client_interface_field.class",
			desc: "Elasticsearch Client is an interface with a static final Setting field initialized in <clinit>. " +
				"If the initializer cannot be hoisted, the field must still render as valid Java instead of being dropped.",
			mustContain: []string{
				"public interface Client",
				"public static final Setting<String> CLIENT_TYPE_SETTING_S = null",
				"public default Client getRemoteClusterClient(String var1)",
			},
			mustNotContain: []string{
				"public static final Setting<String> CLIENT_TYPE_SETTING_S;",
				"decompiled field org.elasticsearch.client.Client.CLIENT_TYPE_SETTING_S",
				"yak-decompiler",
				"post-decompile syntax",
				"undecompilable method body",
			},
		},
		{
			file: "liquibase_co_this_slot_reuse.class",
			desc: "Liquibase JsonNode-style at(ax) reuses local slot 0 as a loop cursor. " +
				"Stores into slot 0 must render as a generated local instead of illegal assignment to this.",
			mustContain: []string{
				"public abstract class co",
				"public final co at(ax var1)",
				"co var0 = this",
				"var0 = var3",
				"return var0",
			},
			mustNotContain: []string{
				"this =",
				"yak-decompiler",
				"post-decompile syntax",
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

// TestDecompileBoolChainShortCircuitMerge locks the short-circuit boolean-materialization fix.
//
// Root cause: javac compiles `boolean u = (c>=A && c<=Z) || c==X || c==Y; if (u) {..} else {..}` into a
// short-circuit chain that materializes u via the iconst_1/goto/iconst_0 idiom into a local slot, then
// `iload; ifeq`. The value-merge ternary rebuilder (buildSharedLeafTernary) only climbed to the root
// condition through SINGLE-source straight-line predecessors. The leading operand `(c>=A && c<=Z)` is a
// compound `&&`, so the next ||-operand's load block has TWO predecessors (the &&-true jump and the
// &&-false fall-through). The climb stopped there and peeled the range check into an OUTER if-statement
// whose then-arm was EMPTY, burying the body and the shared loop increment in the else-arm. For any char
// matching the range the loop then spun forever (no increment, empty arm) - a silent infinite loop that
// ANTLR validation cannot catch.
//
// Fix: when the single-source climb stops at a multi-source reconvergence whose EVERY predecessor is a
// clean ternary condition of this merge (the compound-operand shape), adopt the lowest-id enclosing
// condition so the whole chain is rebuilt as one expression and the if/else has a single full condition.
//
// The structural invariant asserted here is rendering-independent (it holds whether the condition prints
// as a nested ?: or folds to &&/||): there must be NO empty then-arm immediately followed by else (that
// is the infinite-loop signature), and both append calls plus the loop increment must be present.
func TestDecompileBoolChainShortCircuitMerge(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/bool_or_chain_short_circuit.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if strings.Contains(source, "yak-decompiler") {
		t.Fatalf("expected full decompilation, got a stub\n----- source -----\n%s", source)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
	// The bug signature: an if-statement whose then-arm is empty, immediately followed by else. With the
	// fix the boolean is one condition and the then-arm holds the unreserved-char append, so this never
	// matches.
	emptyThenElse := regexp.MustCompile(`\)\s*\{\s*\}\s*else`)
	if emptyThenElse.MatchString(source) {
		t.Fatalf("empty then-arm followed by else (short-circuit boolean mis-merged -> infinite loop)\n----- source -----\n%s", source)
	}
	// Both branch effects and the loop increment must survive on a reachable path.
	for _, must := range []string{"append((char)(var3))", "append((char)(37))"} {
		if !strings.Contains(source, must) {
			t.Fatalf("expected output to contain %q\n----- source -----\n%s", must, source)
		}
	}
	if !regexp.MustCompile(`var\d+\+\+;`).MatchString(source) {
		t.Fatalf("expected a loop increment statement to be present\n----- source -----\n%s", source)
	}
}

// TestDecompileIntStoredBooleanArgCoercion guards the int->boolean argument coercion. The JVM has no
// boolean storage: javac stores/reloads a boolean local with istore/iload and materializes its value
// via iconst_0/iconst_1, so the decompiler types such a local as `int`. Passing it bare to a method
// whose parameter is declared `boolean` fails to recompile ("incompatible types: int cannot be
// converted to boolean"). coerceBooleanArgument now wraps a genuinely int-typed value flowing into a
// boolean parameter as `(v) != (0)` (semantically exact: a boolean is int 0/1 on the stack) while
// leaving already-boolean values untouched (never emitting the illegal `(a > b) != (0)`). This is a
// behavior/recompile bug the ANTLR syntax safety net cannot catch, so we assert it structurally.
func TestDecompileIntStoredBooleanArgCoercion(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/bool_int_to_boolean_arg.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if strings.Contains(source, javaclassparser.DecompileStubMarker) {
		t.Fatalf("expected full decompilation, got a stub\n----- source -----\n%s", source)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
	// The int-typed boolean local (prev) must be coerced at the boolean-parameter sink:
	// pred(c, prev) -> pred(varX, (varY) != (0)). Without the coercion the bare int arg fails javac.
	coerced := regexp.MustCompile(`pred\(var\d+,\s*\(var\d+\) != \(0\)\)`)
	if !coerced.MatchString(source) {
		t.Fatalf("expected int->boolean argument coercion `(v) != (0)` at the boolean-parameter call site\n----- source -----\n%s", source)
	}
	// Over-coercion guard: pred's own boolean parameter (var1) is already boolean and must NOT be
	// wrapped (that would emit the illegal `boolean != int`). Its use in the short-circuit return
	// stays bare.
	if strings.Contains(source, "(var1) != (0)") {
		t.Fatalf("boolean-typed value was wrongly coerced with `!= (0)` (over-coercion -> illegal boolean!=int)\n----- source -----\n%s", source)
	}
}

// TestDecompileSwitchDescendingFallthrough guards descending-value `switch` fall-through (the Murmur3
// finalization tail: `case 3: ...; case 2: ...; case 1: ...;` with no breaks). The cases intentionally
// fall through in DESCENDING order; an earlier defect re-sorted switch cases ascending by label, which
// inverts fall-through direction and silently corrupts the result (e.g. array out-of-bounds / wrong
// digest) while still parsing as valid Java. We assert the case labels AND their bodies stay in
// descending order so an ascending re-sort fails the build.
func TestDecompileSwitchDescendingFallthrough(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/switch_descending_fallthrough.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}
	source, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if strings.Contains(source, javaclassparser.DecompileStubMarker) {
		t.Fatalf("expected full decompilation, got a stub\n----- source -----\n%s", source)
	}
	if _, ferr := java2ssa.Frontend(source); ferr != nil {
		t.Fatalf("frontend parse failed: %v\n----- source -----\n%s", ferr, source)
	}
	// Case labels must appear in descending order: case 3, then case 2, then case 1.
	for _, seq := range [][2]string{{"case 3:", "case 2:"}, {"case 2:", "case 1:"}} {
		a, b := strings.Index(source, seq[0]), strings.Index(source, seq[1])
		if a < 0 || b < 0 || a >= b {
			t.Fatalf("switch case labels not in descending order (%q before %q expected)\n----- source -----\n%s", seq[0], seq[1], source)
		}
	}
	// Case BODIES must also stay in descending order: data[2] (case 3) before data[1] (case 2) before
	// data[0] (case 1). An ascending re-sort would emit data[0] first, inverting fall-through.
	i2, i1, i0 := strings.Index(source, "var0[2]"), strings.Index(source, "var0[1]"), strings.Index(source, "var0[0]")
	if i2 < 0 || i1 < 0 || i0 < 0 || !(i2 < i1 && i1 < i0) {
		t.Fatalf("descending fall-through bodies out of order (expected data[2] < data[1] < data[0])\n----- source -----\n%s", source)
	}
}

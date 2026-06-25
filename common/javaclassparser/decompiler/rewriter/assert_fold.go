package rewriter

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
)

// assertFoldOff reads the ASSERT_FOLD_OFF kill-switch once per process. Mirrors the style of the
// existing JSR_INLINE_OFF / EnableLegacyMergeReconstruction toggles so the assert-guard fold can
// be reverted without a code change if it ever mis-folds.
var assertFoldOff = os.Getenv("ASSERT_FOLD_OFF") != ""

// foldAssertionGuards rewrites the assertion-guard idiom into a syntactically-valid form so it
// survives post-decompile syntax validation instead of degrading the whole method to a stub.
//
// javac lowers `assert <cond>;` into roughly:
//
//	if (!$assertionsDisabled) {                 // guard: assertions disabled => skip
//	    if (!(<cond>)) throw new AssertionError(...);
//	}
//
// On a single assert this already reconstructs fine. But when several asserts share/overlap
// throw targets, the value-merge structuring can leave an orphaned ConditionStatement immediately
// followed by its `throw new AssertionError()`:
//
//	<...inside a ternary arm...>
//	    if (!($assertionsDisabled)) && ((this.elements[...]) != (null));   // <-- orphaned guard, FATAL
//	    throw new AssertionError();
//
// The `if (cond);` (a bare ConditionStatement rendered as a top-level statement) is not valid
// Java, so the safety net stubs the whole method. This pass detects that orphaned pair — a
// ConditionStatement whose condition mentions `$assertionsDisabled`, immediately followed (or
// wrapping) a `throw new AssertionError(...)` — and folds the throw into a real IfStatement body:
//
//	if (!($assertionsDisabled) && (<cond>)) {
//	    throw new AssertionError(...);
//	}
//
// It only acts on the exact corrupted shape, so already-correctly-structured asserts and ordinary
// code are left byte-for-byte untouched. This is a conservative repair: it never changes semantics
// (the throw already executed on that path), it just gives the orphaned condition a valid body.
//
// kill-switch: ASSERT_FOLD_OFF=1 reverts to the old behavior (let it stub).
func FoldAssertionGuards(sts []statements.Statement) []statements.Statement {
	if assertFoldOff {
		return sts
	}
	out := make([]statements.Statement, 0, len(sts))
	for _, st := range sts {
		out = append(out, foldAssertionGuardsInStatement(st))
	}
	return foldAssertionGuardPairs(out)
}

// foldAssertionGuardsInStatement recurses into the nested bodies of composite statements.
func foldAssertionGuardsInStatement(st statements.Statement) statements.Statement {
	switch s := st.(type) {
	case *statements.IfStatement:
		s.IfBody = FoldAssertionGuards(s.IfBody)
		s.ElseBody = FoldAssertionGuards(s.ElseBody)
	case *statements.DoWhileStatement:
		s.Body = FoldAssertionGuards(s.Body)
	case *statements.WhileStatement:
		s.Body = FoldAssertionGuards(s.Body)
	case *statements.ForStatement:
		s.SubStatements = FoldAssertionGuards(s.SubStatements)
	case *statements.SwitchStatement:
		for _, c := range s.Cases {
			c.Body = FoldAssertionGuards(c.Body)
		}
	case *statements.TryCatchStatement:
		s.TryBody = FoldAssertionGuards(s.TryBody)
		for i := range s.CatchBodies {
			s.CatchBodies[i] = FoldAssertionGuards(s.CatchBodies[i])
		}
	case *statements.SynchronizedStatement:
		s.Body = FoldAssertionGuards(s.Body)
	}
	return st
}

// foldAssertionGuardPairs scans a flat statement list for the orphaned
// `ConditionStatement(mentions $assertionsDisabled)` + `throw AssertionError()` pair and folds the
// throw into a real if-body. A pair is only folded when BOTH conditions hold: the condition
// mentions $assertionsDisabled (so this is definitively an assert-guard, not arbitrary code) and
// the next statement is a throw of AssertionError.
func foldAssertionGuardPairs(sts []statements.Statement) []statements.Statement {
	if len(sts) < 2 {
		return sts
	}
	out := make([]statements.Statement, 0, len(sts))
	for i := 0; i < len(sts); i++ {
		cond, isCond := sts[i].(*statements.ConditionStatement)
		if isCond && i+1 < len(sts) && mentionsAssertionsDisabled(cond.Condition) && isThrowAssertionError(sts[i+1]) {
			out = append(out, &statements.IfStatement{
				Condition: cond.Condition,
				IfBody:    []statements.Statement{sts[i+1]},
			})
			i++ // consume the throw
			continue
		}
		out = append(out, sts[i])
	}
	return out
}

// mentionsAssertionsDisabled reports whether the rendered form of v contains the synthetic
// $assertionsDisabled identifier. Rendering is cheap (a single value) and is the only reliable way
// to spot the field reference regardless of how the stack simulation wrapped it.
func mentionsAssertionsDisabled(v values.JavaValue) bool {
	if v == nil {
		return false
	}
	return strings.Contains(v.String(&class_context.ClassContext{}), "$assertionsDisabled")
}

// isThrowAssertionError reports whether st is a `throw new AssertionError(...)` statement. The
// decompiler renders throws as a CustomStatement whose text starts with "throw "; an
// AssertionError throw's text also contains "AssertionError".
func isThrowAssertionError(st statements.Statement) bool {
	c, ok := st.(*statements.CustomStatement)
	if !ok {
		return false
	}
	txt := strings.TrimSpace(c.String(&class_context.ClassContext{}))
	return strings.HasPrefix(txt, "throw ") && strings.Contains(txt, "AssertionError")
}

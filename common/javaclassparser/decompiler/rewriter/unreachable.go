package rewriter

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
)

// pruneCtx is a throwaway rendering context used only to inspect the textual form of
// the opaque CustomStatement break/continue/throw markers. It carries no state.
var pruneCtx = &class_context.ClassContext{}

// PruneUnreachableStatements removes statements that follow a statement which cannot
// complete normally (a "terminal" statement) within the same block. javac rejects
// such trailing code as an "unreachable statement"; the structuring pass can emit it
// when, for example, a back-edge `continue` is placed after an inner infinite loop
// that only ever exits via `return` or a labelled `continue` to an outer loop.
//
// The terminal classification is a deliberately strict subset of the JLS
// "cannot complete normally" rules (return / throw / break / continue, an if/else
// whose branches are both terminal, and an infinite `while(true)`/`do{...}while(true)`
// with no break that could fall through). Because it is a subset, the pass only ever
// deletes code that javac also considers unreachable, so a class that already
// recompiles is left byte-for-byte identical and no reachable code is dropped.
func PruneUnreachableStatements(sts []statements.Statement) []statements.Statement {
	for _, st := range sts {
		pruneInsideStatement(st)
	}
	for i, st := range sts {
		if statementIsTerminal(st) {
			return sts[:i+1]
		}
	}
	return sts
}

// pruneInsideStatement recurses the prune into every nested block of a composite
// statement. It must run before the block-level cut so terminality of a nested
// construct is judged on its already-pruned body.
func pruneInsideStatement(st statements.Statement) {
	switch s := st.(type) {
	case *statements.IfStatement:
		s.IfBody = PruneUnreachableStatements(s.IfBody)
		s.ElseBody = PruneUnreachableStatements(s.ElseBody)
	case *statements.DoWhileStatement:
		s.Body = PruneUnreachableStatements(s.Body)
	case *statements.WhileStatement:
		s.Body = PruneUnreachableStatements(s.Body)
	case *statements.ForStatement:
		s.SubStatements = PruneUnreachableStatements(s.SubStatements)
	case *statements.SwitchStatement:
		for _, c := range s.Cases {
			c.Body = PruneUnreachableStatements(c.Body)
		}
	case *statements.TryCatchStatement:
		s.TryBody = PruneUnreachableStatements(s.TryBody)
		for i := range s.CatchBodies {
			s.CatchBodies[i] = PruneUnreachableStatements(s.CatchBodies[i])
		}
	case *statements.SynchronizedStatement:
		s.Body = PruneUnreachableStatements(s.Body)
	}
}

// statementIsTerminal reports whether control can NOT fall through past this
// statement to the next statement in the same block.
func statementIsTerminal(st statements.Statement) bool {
	switch s := st.(type) {
	case *statements.ReturnStatement:
		return true
	case *statements.CustomStatement:
		txt := strings.TrimSpace(s.String(pruneCtx))
		return txt == "break" || strings.HasPrefix(txt, "break ") ||
			txt == "continue" || strings.HasPrefix(txt, "continue ") ||
			strings.HasPrefix(txt, "throw ")
	case *statements.IfStatement:
		if len(s.IfBody) == 0 || len(s.ElseBody) == 0 {
			return false
		}
		return blockIsTerminal(s.IfBody) && blockIsTerminal(s.ElseBody)
	case *statements.DoWhileStatement:
		return isLiteralTrue(s.ConditionValue) && !subtreeHasBreak(s.Body)
	case *statements.WhileStatement:
		return isLiteralTrue(s.ConditionValue) && !subtreeHasBreak(s.Body)
	}
	return false
}

// blockIsTerminal reports whether the last statement of a (already-pruned) block is
// terminal, i.e. the block as a whole cannot complete normally.
func blockIsTerminal(sts []statements.Statement) bool {
	if len(sts) == 0 {
		return false
	}
	return statementIsTerminal(sts[len(sts)-1])
}

func isLiteralTrue(v values.JavaValue) bool {
	lit, ok := v.(*values.JavaLiteral)
	if !ok {
		return false
	}
	return fmt.Sprint(lit.Data) == "true"
}

// subtreeHasBreak over-approximates "this loop can fall through": it reports true if
// any break-like marker appears anywhere in the body subtree. Over-detecting breaks
// (e.g. a break that actually targets a nested switch or an outer label) only makes
// the loop look non-terminal, which suppresses pruning — never the other way round —
// so it can never delete reachable code.
func subtreeHasBreak(sts []statements.Statement) bool {
	for _, st := range sts {
		if statementSubtreeHasBreak(st) {
			return true
		}
	}
	return false
}

func statementSubtreeHasBreak(st statements.Statement) bool {
	switch s := st.(type) {
	case *statements.CustomStatement:
		txt := strings.TrimSpace(s.String(pruneCtx))
		return txt == "break" || strings.HasPrefix(txt, "break ")
	case *statements.IfStatement:
		return subtreeHasBreak(s.IfBody) || subtreeHasBreak(s.ElseBody)
	case *statements.DoWhileStatement:
		return subtreeHasBreak(s.Body)
	case *statements.WhileStatement:
		return subtreeHasBreak(s.Body)
	case *statements.ForStatement:
		return subtreeHasBreak(s.SubStatements)
	case *statements.SwitchStatement:
		for _, c := range s.Cases {
			if subtreeHasBreak(c.Body) {
				return true
			}
		}
		return false
	case *statements.TryCatchStatement:
		if subtreeHasBreak(s.TryBody) {
			return true
		}
		for _, b := range s.CatchBodies {
			if subtreeHasBreak(b) {
				return true
			}
		}
		return false
	case *statements.SynchronizedStatement:
		return subtreeHasBreak(s.Body)
	}
	return false
}

package ssa

import (
	"reflect"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

// DetachAST cuts node's upward parent pointer (ANTLR BaseRuleContext.parentCtx)
// and returns node unchanged for convenient chaining.
//
// Why this exists:
//
// Lazy build closures capture a body subtree (a function body, method body or a
// file-level statement block). ANTLR's runtime gives every node a parent
// pointer (rule_context.go: BaseRuleContext.parentCtx). So a closure that
// captures even a single small subtree transitively pins the ENTIRE file parse
// tree alive through parentCtx -> ... -> file root. Detaching the captured
// subtree's parent pointer breaks that chain: once the file root is no longer
// referenced elsewhere, only the detached, self-contained subtrees survive and
// the rest of the tree becomes collectable.
//
// Why we do NOT remove node from its parent's children slice:
//
//   - ANTLR's BaseParserRuleContext.children is unexported and exposes no
//     by-reference remover (only AddChild / RemoveLastChild), so removing a
//     specific child via the public API is impossible.
//   - It is also unnecessary: the parent itself becomes unreachable once the
//     file root reference is dropped, so its children slice is collected
//     together with it.
//   - It would be actively harmful: several front ends walk parent.children
//     more than once (e.g. go2ssa calls ast.AllFunctionDecl() twice for
//     init-vs-normal functions). Mutating children mid-pass would corrupt those
//     repeated walks.
//
// Cutting parentCtx alone is therefore both sufficient and safe. This relies on
// the invariant (verified across all language builders) that lazy body builds
// never call GetParent()/walk upward; they only descend into their own subtree.
func DetachAST[T antlr.Tree](node T) T {
	detachParent(node)
	return node
}

func detachParent(node antlr.Tree) {
	if node == nil {
		return
	}
	// Guard against a typed-nil wrapped in a non-nil interface, which would
	// otherwise nil-deref inside SetParent.
	if rv := reflect.ValueOf(node); rv.Kind() == reflect.Ptr && rv.IsNil() {
		return
	}
	// Only ParserRuleContext nodes pin the parse tree via parentCtx in a way that
	// matters here, and only their SetParent (BaseRuleContext) is nil-safe.
	// TerminalNode.SetParent panics on a nil argument and terminals are never
	// captured by a lazy builder, so they are intentionally skipped.
	if prc, ok := node.(antlr.ParserRuleContext); ok {
		prc.SetParent(nil)
	}
}

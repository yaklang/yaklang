package ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

// DetachAST slims a retained ANTLR subtree and returns node unchanged for
// convenient chaining.
//
// Lazy build closures capture small parser subtrees such as function bodies,
// parameter lists, field declarators, and file-level statements. Without
// slimming, those subtrees keep the entire file parse alive through parent
// pointers, generated ctx.parser fields, parser caches, and token
// source/input-stream references.
//
// The slimmed subtree keeps generated context types, children, token text, and
// token offsets. Visitors can still descend through the subtree and compute SSA
// ranges from the already-retained editor, but the subtree no longer pins the
// full source input or parser graph. Lazy builders must not rely on walking
// upward through GetParent().
func DetachAST[T antlr.Tree](node T) T {
	return antlr4util.SlimParserTree(node)
}

// DetachASTRootChildren cheaply cuts a file root's direct child links after all
// lazy-retained subtrees have already been captured with DetachAST.
//
// It intentionally does not recursively slim the whole file tree and does not
// copy token text. This keeps file-level cleanup from adding large temporary
// allocations for AST nodes that are about to become unreachable anyway.
func DetachASTRootChildren(node antlr.Tree) {
	antlr4util.DetachParserTreeChildren(node)
}

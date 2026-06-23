package core

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
)

// RewriteNewArrayList folds the explicit sized-array-then-fill idiom into an array literal:
//
//	int[] a = new int[N];   // assign node
//	a[0] = v0; ...; a[N-1] = vN-1;  // N sequential element nodes
//
// It records the elements into the NewExpression initializer (so the array renders as
// `new T[]{v0,...}` and the element stores are not lost) and removes the element nodes from the graph
// by relinking the assign node straight to the statement that follows the fill. The graph surgery is
// load-bearing: leaving the element-assign nodes in place leaves dangling fall-through edges that the
// downstream CFG structuring rejects with "multiple next". Only literals whose length is a constant
// and whose indices are exactly 0..N-1 in order qualify, so sparse/partial fills are left untouched.
//
// Both the inline / returned literal (javac's dup-temporary, which is assigned to a synthetic local
// before this pass runs) and the form explicitly stored to a named local funnel through the same
// `assign = new T[N]` + sequential element-store shape here, so a single post-pass covers them. The
// fold must run as a post-pass rather than during emission because suppressing the element-store
// opcodes inline corrupts jump targets that land on them; the relinking here preserves the CFG.
func RewriteNewArrayList(node *Node, delMap map[string][3]int) {
	if len(node.Next) != 1 {
		return
	}
	st, ok := node.Statement.(*statements.AssignStatement)
	if !ok {
		return
	}
	refVal, ok := st.LeftValue.(*values.JavaRef)
	if !ok {
		return
	}
	newExp, ok := st.JavaValue.(*values.NewExpression)
	if !ok {
		return
	}
	if len(newExp.Length) == 0 {
		return
	}
	arrayLengthVar := newExp.Length[0]
	lVar, ok := arrayLengthVar.(*values.JavaLiteral)
	if !ok {
		return
	}
	lvar1, ok := lVar.Data.(int)
	if !ok {
		return
	}
	next := node.Next[0]
	vs := []values.JavaValue{}
	for i := 0; i < lvar1; i++ {
		if len(next.Next) != 1 {
			return
		}
		asEleSt, ok := next.Statement.(*statements.AssignStatement)
		if !ok {
			return
		}
		if asEleSt.ArrayMember == nil {
			return
		}
		if st.LeftValue != UnpackSoltValue(asEleSt.ArrayMember.Object) {
			return
		}
		lVar, ok := asEleSt.ArrayMember.Index.(*values.JavaLiteral)
		if !ok {
			return
		}
		lvar1, ok := lVar.Data.(int)
		if !ok {
			return
		}
		if lvar1 != i {
			return
		}
		vs = append(vs, asEleSt.JavaValue)
		next = next.Next[0]
	}
	attr := delMap[refVal.VarUid]
	attr[0]++
	delMap[refVal.VarUid] = attr
	newExp.Initializer = vs
	node.RemoveAllNext()
	node.AddNext(next)
}

func MiscRewriter(rootNode *Node, delMap map[string][3]int) error {
	WalkGraph[*Node](rootNode, func(n *Node) ([]*Node, error) {
		RewriteNewArrayList(n, delMap)
		return n.Next, nil
	})
	return nil
}

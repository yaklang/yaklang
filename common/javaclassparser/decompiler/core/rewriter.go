package core

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
)

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
	//newExp.Initializer = vs
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

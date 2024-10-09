package rewriter

import (
	"errors"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)

func CheckNodesIsValid(node *core.Node) error {
	err := core.WalkGraph[*core.Node](node, func(node *core.Node) ([]*core.Node, error) {
		if node.Statement == nil {
			return nil, errors.New("statement is nil")
		}
		if _, ok := node.Statement.(*core.ConditionStatement); ok {
			if len(node.Next) != 2 {
				return nil, errors.New("if statement must have two next node")
			}
		}
		return node.Next, nil
	})
	return err
}

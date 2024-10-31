package rewriter

import (
	"errors"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
)

func LabelRewriter(manager *StatementManager) error {
	for _, pair := range manager.UncertainBreakNodes {
		node := pair[0]
		to := pair[1]
		finalNode := to
		for finalNode.HideNext != nil {
			finalNode = finalNode.HideNext
		}
		matched := utils.NodeFilter(manager.WhileNode, func(node *core.Node) bool {
			return node == finalNode
		})
		if len(matched) > 0 {
			if utils.IsDominate(manager.DominatorMap, matched[0], node) {
				loopNode := matched[0].Statement.(*statements.DoWhileStatement)
				if loopNode.Label == "" {
					label := manager.NewLoopLabel()
					loopNode.Label = label
				}
				to.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
					return "continue " + loopNode.Label
				})
			}
			//} else {
			//	return errors.New("loop end node conflict")
			//}
		} else {
			if node.LoopEndNode == nil {
				to.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
					return "break"
				})
				continue
			}
			var ok bool
			for _, n := range manager.WhileNode {
				if n.LoopEndNode == finalNode && utils.IsDominate(manager.DominatorMap, n, node) {
					loopNode := n.Statement.(*statements.DoWhileStatement)
					if loopNode.Label == "" {
						label := manager.NewLoopLabel()
						loopNode.Label = label
					}
					to.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
						return "break " + loopNode.Label
					})
					ok = true
					break
				}
			}
			if !ok {
				return errors.New("loop end node conflict")
			}
		}
	}
	return nil
}

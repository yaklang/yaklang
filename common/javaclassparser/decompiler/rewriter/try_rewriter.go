package rewriter

//func TryRewriter(manager *RewriteManager) error {
//	for _, node := range manager.TryNodes {
//		leftSet := utils.NewSet[*core.Node]()
//		core.WalkGraph[*core.Node](node.Next[0], func(node *core.Node) ([]*core.Node, error) {
//			leftSet.Add(node)
//			return node.Next, nil
//		})
//		var mergeNode *core.Node
//		core.WalkGraph[*core.Node](node.Next[1], func(node *core.Node) ([]*core.Node, error) {
//			if leftSet.Has(node) {
//				mergeNode = node
//				return nil, nil
//			}
//			return node.Next, nil
//		})
//		node := node
//		//if mergeNode == nil {
//		//	return errors.New("try rewriter error")
//		//}
//		next := make([]*core.Node, len(node.Next))
//		copy(next, node.Next)
//		//node.RemoveAllNext()
//		//
//		//tryCatchNode := manager.NewNode(tryCatchSt)
//		//node.AddNext(tryCatchNode)
//		tryCatchSt := statements.NewTryCatchStatement(nil, nil)
//		node.Statement = tryCatchSt
//		node.RemoveAllNext()
//		if mergeNode != nil {
//			node.AddNext(mergeNode)
//		}
//		manager.AddFinalAction(func() error {
//			tryBody, err := manager.ToStatementsFromNode(next[0], func(node *core.Node) bool {
//				if mergeNode != nil && node == mergeNode {
//					return false
//				}
//				return true
//			})
//			if err != nil {
//				return err
//			}
//			tryCatchSt.TryBody = core.NodesToStatements(tryBody)
//			for _, c := range next {
//				catchBody, err := manager.ToStatementsFromNode(c, func(node *core.Node) bool {
//					if mergeNode != nil && node == mergeNode {
//						return false
//					}
//					return true
//				})
//				if err != nil {
//					return err
//				}
//				tryCatchSt.Exception = catchBody[0].Statement.(*statements.AssignStatement).LeftValue.(*values.JavaRef)
//				tryCatchSt.CatchBodies = append(tryCatchSt.CatchBodies, core.NodesToStatements(catchBody)[1:])
//			}
//			return nil
//		})
//	}
//	return nil
//}

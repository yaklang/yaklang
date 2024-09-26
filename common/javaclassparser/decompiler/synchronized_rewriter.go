package decompiler

func SynchronizedRewriter(manager *StatementManager, node *Node) error {
	if err := manager.ScanStatementSimple(func(node *Node) error {
		cStem, ok := node.Statement.(*CustomStatement)
		if !ok {
			return nil
		}
		if cStem.Name != "monitor_enter" {
			return nil
		}
		monitorValue := cStem.Info.(JavaValue)
		monitorManger := NewStatementManager(node.Next[0])
		var exitNode *Node
		err := monitorManger.Rewrite(func(node *Node) bool {
			if len(node.Next) == 0 {
				return true
			}
			nextNode := node.Next[0]
			cStem, ok := nextNode.Statement.(*CustomStatement)
			if ok && cStem.Name == "monitor_exit" {
				exitNode = nextNode
				return false
			}
			return true
		})
		if err != nil {
			return err
		}
		if exitNode == nil {
			return nil
		}
		body, err := monitorManger.ToStatements()
		if err != nil {
			return err
		}
		node.Statement = NewSynchronizedStatement(monitorValue, body)
		node.Next = exitNode.Next
		if _, ok := exitNode.Next[0].Statement.(*GOTOStatement); ok {
			node.Next = exitNode.Next[0].Next
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

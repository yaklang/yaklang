package aid

import "github.com/yaklang/yaklang/common/utils/linktable"

func DFSOrderAiTask(root *AiTask) *linktable.LinkedList[*AiTask] {
	result := linktable.New[*AiTask]()

	treeStack := []*AiTask{root}
	for len(treeStack) > 0 {
		// Pop a node from the treeStack.
		lastIndex := len(treeStack) - 1
		currentTask := treeStack[lastIndex]
		treeStack = treeStack[:lastIndex]
		result.PushBack(currentTask)
		children := currentTask.Subtasks
		for i := len(children) - 1; i >= 0; i-- {
			treeStack = append(treeStack, children[i])
		}
	}

	return result
}

// DFSOrderAiTaskPostOrder 使用后序遍历（Post-order）来遍历 AiTask 树。
// 遍历顺序：先访问所有子任务，最后访问父任务。
func DFSOrderAiTaskPostOrder(root *AiTask) *linktable.LinkedList[*AiTask] {
	result := linktable.New[*AiTask]()
	if root == nil {
		return result
	}

	treeStack := make([]*AiTask, 0)
	var lastVisited *AiTask // 用于记录上一个被访问（加入result）的节点

	// 从根节点开始
	treeStack = append(treeStack, root)

	for len(treeStack) > 0 {
		// 查看（不弹出）栈顶元素
		peekNode := treeStack[len(treeStack)-1]

		// 检查是否应该访问当前节点（peekNode）。
		// 满足以下任一条件即可访问：
		// 1. peekNode 是叶子节点（没有子任务）。
		// 2. 上一个访问的节点 (lastVisited) 是 peekNode 的最后一个子任务。
		//    这表示 peekNode 的所有子树都已经被处理完毕。

		isLeaf := len(peekNode.Subtasks) == 0
		allChildrenVisited := !isLeaf && lastVisited == peekNode.Subtasks[len(peekNode.Subtasks)-1]

		if isLeaf || allChildrenVisited {
			// 访问并弹出节点
			result.PushBack(peekNode)
			treeStack = treeStack[:len(treeStack)-1] // Pop from stack
			lastVisited = peekNode                   // 更新 lastVisited
		} else {
			// 将子节点（从右到左）压入栈中
			// 这样能保证出栈时是“从左到右”的顺序
			for i := len(peekNode.Subtasks) - 1; i >= 0; i-- {
				treeStack = append(treeStack, peekNode.Subtasks[i])
			}
		}
	}

	return result
}

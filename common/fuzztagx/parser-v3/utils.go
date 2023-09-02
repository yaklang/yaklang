package parser

type trieNode struct {
	children   map[rune]*trieNode
	failure    *trieNode
	output     bool
	patternLen int
	id         int
}

// IndexAllSubstrings 只遍历一次查找所有子串位置
// 返回值是一个二维数组，每个元素是一个[2]int类型匹配结果，其中第一个元素是规则index，第二个元素是索引位置
func IndexAllSubstrings(s string, patterns ...string) (result [][2]int) {
	// 构建trie树
	root := &trieNode{
		children:   make(map[rune]*trieNode),
		failure:    nil,
		output:     false,
		patternLen: 0,
	}

	for patternIndex, pattern := range patterns {
		node := root
		for _, char := range pattern {
			if _, ok := node.children[char]; !ok {
				node.children[char] = &trieNode{
					children:   make(map[rune]*trieNode),
					failure:    nil,
					output:     false,
					patternLen: 0,
					id:         patternIndex,
				}
			}
			node = node.children[char]
		}
		node.output = true
		node.patternLen = len(pattern)
	}
	// 构建Failure
	queue := make([]*trieNode, 0)
	root.failure = root

	for _, child := range root.children {
		child.failure = root
		queue = append(queue, child)
	}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		for char, child := range node.children {
			queue = append(queue, child)
			failure := node.failure

			for failure != root && failure.children[char] == nil {
				failure = failure.failure
			}

			if next := failure.children[char]; next != nil {
				child.failure = next
				child.output = child.output || next.output
			} else {
				child.failure = root
			}
		}
	}

	// 查找
	node := root
	for i, char := range s {
		for node != root && node.children[char] == nil {
			node = node.failure
		}

		if next := node.children[char]; next != nil {
			node = next
			if node.output {
				result = append(result, [2]int{node.id, i - node.patternLen + 1})
			}
		}
	}
	return
}

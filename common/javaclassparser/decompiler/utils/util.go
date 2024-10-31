package utils

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)

func LinkNode(src, target *core.Node) {
	target.Source = append(target.Source, src)
	src.Next = append(src.Next, target)
}

func NodeFilter(nodes []*core.Node, f func(node *core.Node) bool) []*core.Node {
	var res []*core.Node
	for _, node := range nodes {
		if f(node) {
			res = append(res, node)
		}
	}
	return res
}

func IsDominate(tree map[*core.Node][]*core.Node, node1, node2 *core.Node) bool {
	if node1 == node2 {
		return true
	}
	doms := tree[node1]
	for _, dom := range doms {
		if dom == node2 {
			return true
		} else {
			if IsDominate(tree, dom, node2) {
				return true
			}
		}
	}
	return false
}

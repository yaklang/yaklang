package base

import "bytes"

const (
	CfgParent   = "parent"
	CfgLastNode = "lastNode"
	CfgRootMap  = "rootNodeMap"
)

var parseMap = make(map[string]Parser)

func RegisterParser(name string, parser Parser) {
	parseMap[name] = parser
}

type Parser interface {
	Parse(data *BitReader, node *Node) error
	Generate(data any, node *Node) error
	OnRoot(node *Node) error
}
type BaseParser struct {
	root *Node
}

// OnRoot 设置了Ctx: root、rootNodeMap; Cfg：parent、lastNode,writer,buffer
func (b *BaseParser) OnRoot(rootNode *Node) error {
	rootNode.Ctx.SetItem("root", rootNode)
	buffer := &bytes.Buffer{}
	rootNode.Ctx.SetItem("buffer", buffer)
	rootNode.Ctx.SetItem("writer", NewBitWriter(buffer))
	rootChildMap := make(map[string]*Node)
	var walkNode func(node *Node) error
	walkNode = func(node *Node) error {
		for i, child := range node.Children {
			if node == rootNode {
				rootChildMap[child.Name] = child
			}
			err := walkNode(child)
			if err != nil {
				return err
			}
			if i == len(node.Children)-1 {
				child.Cfg.SetItem(CfgLastNode, true)
			}
			child.Cfg.SetItem(CfgParent, node)
		}
		return nil
	}
	rootNode.Ctx.SetItem(CfgRootMap, rootChildMap)
	return walkNode(rootNode)
}

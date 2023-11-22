package base

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


package base

var parseMap = make(map[string]Parser)

func RegisterParser(name string, parser Parser) {
	parseMap[name] = parser
}

type Parser interface {
	Parse(data *BitReader, node *Node) (*NodeResult, error)
	Generate(data any, node *Node) (*NodeResult, error)
	OnRoot(node *Node) error
}
type BaseParser struct {
	root *Node
}

func (b *BaseParser) OnRoot(node *Node) error {
	return nil
}

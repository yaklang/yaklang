package fuzztagx

type Node interface {
	Strings() []string
}

// String
type StringNode string

func NewStringNode(s string) *StringNode {
	n := StringNode(s)
	return &n
}
func (s *StringNode) Strings() []string {
	return []string{string(*s)}
}

// String
type ExpressionNode string

func NewExpressionNode(s string) *ExpressionNode {
	n := ExpressionNode(s)
	return &n
}
func (s *ExpressionNode) Strings() []string {
	return []string{string(*s)}
}

// FuzzTag/ExpressionTag
type Tag struct {
	IsExpTag bool
	Nodes []Node
}

func (f *Tag) Strings() []string {
	return nil
}

// FuzzTagMethod
type FuzzTagMethod struct {
	name  string
	params []Node
}

func (f *FuzzTagMethod) Strings() []string {
	return nil
}

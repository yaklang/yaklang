package fuzztagx

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"strings"
)

type NodeAttr struct {
	index int
	isDyn bool
	isRep bool
}
type Node interface {
	Strings() []string
	GenerateOne() (string, bool)
	Reset()
}

// String
type StringNode struct {
	*NodeAttr
	data string
}

func NewStringNode(s string) *StringNode {
	return &StringNode{
		NodeAttr: &NodeAttr{
			isDyn: false,
			isRep: true,
		},
		data: s,
	}
}
func (s *StringNode) Strings() []string {
	return []string{s.data}
}
func (s *StringNode) GenerateOne() (string, bool) {
	if s.index > 0 {
		return "", false
	} else {
		s.index++
		return s.data, true
	}
}
func (s *StringNode) Reset() {
	s.index = 0
}

// Expression
type ExpressionNode struct {
	*NodeAttr
	data string
}

func NewExpressionNode(s string) *ExpressionNode {
	return &ExpressionNode{
		NodeAttr: &NodeAttr{
			isDyn: false,
			isRep: false,
		},
		data: s,
	}
}
func (s *ExpressionNode) Strings() []string {
	return []string{s.data}
}
func (f *ExpressionNode) GenerateOne() (string, bool) {
	if f.index > 0 {
		return "", false
	} else {
		box := httptpl.NewNucleiDSLYakSandbox()
		res, err := box.Execute(f.data)
		if err != nil {
			return "", true
		} else {
			return utils.InterfaceToString(res), true
		}
	}
}
func (s *ExpressionNode) Reset() {
	s.index = 0
}

// FuzzTag/ExpressionTag
type Tag struct {
	IsExpTag  bool
	Nodes     []Node
	generator *Generator
}

func (f *Tag) Strings() []string {
	return nil
}
func (f *Tag) GenerateOne() (string, bool) {
	if f.generator == nil {
		f.generator = NewGenerator(f.Nodes)
	}
	return f.generator.Generate()
}
func (s *Tag) Reset() {
	s.generator = nil
	for _, node := range s.Nodes {
		node.Reset()
	}
}

// FuzzTagMethod
type FuzzTagMethod struct {
	cache     *[]string
	name      string
	label     string
	isDyn     bool
	isRep     bool
	params    []Node
	funTable  *map[string]BuildInTagFun
	generator *Generator
	index     int
}

func (f *FuzzTagMethod) Strings() []string {
	return nil
}
func (f *FuzzTagMethod) ParseLabel() {
	labels := strings.Split(f.name, "::")
	splits := strings.Split(labels[0], "-")
	f.name = splits[0]
	for _, s := range splits[1:] {
		switch s {
		case "dyn":
			f.isDyn = true
		case "rep":
			f.isRep = true
		}
	}
	for _, label := range labels[1:] {
		f.label = label
	}
}

func (f *FuzzTagMethod) GenerateOne() (string, bool) {
	if f.generator == nil { // 未初始化
		f.generator = NewGenerator(f.params)
		f.ParseLabel()
	}
CHECK:
	if f.cache == nil || f.isDyn || f.index >= len(*f.cache) {
		if f.funTable != nil && *f.funTable != nil {
			fun, ok := (*f.funTable)[f.name]
			if !ok {
				return "", true
			}
			s, ok := f.generator.Generate()
			if !ok {
				return "", false
			}
			result := fun(s)
			f.cache = &result
			if f.index >= len(*f.cache) {
				f.index = 0
			}
			goto CHECK
		} else {
			return "", false
		}
	}

	//if f.index >= len(*f.cache) {
	//	if !f.isRep {
	//		return "", false
	//	} else {
	//		return utils.GetLastElement(*f.cache), true
	//	}
	//}
	defer func() {
		f.index++
	}()
	return (*f.cache)[f.index], true

}
func (s *FuzzTagMethod) Reset() {
	s.generator = nil
	s.index = 0
	s.cache = nil
	for _, param := range s.params {
		param.Reset()
	}
}

type Generator struct {
	container []string
	//index     int
	data  []Node
	first bool
}

func NewGenerator(nodes []Node) *Generator {
	g := &Generator{data: nodes, container: make([]string, len(nodes)), first: true}
	for index, d := range g.data {
		s, _ := d.GenerateOne()
		g.container[index] = s
	}
	return g
}
func (g *Generator) Generate() (string, bool) {
	if g.first {
		defer func() {
			g.first = false
		}()
		return strings.Join(g.container, ""), true
	} else {
		isOk := false
		i := 0
		for {
			if len(g.data) == i {
				break
			}
			s, ok := g.data[i].GenerateOne()
			if !ok {
				if i < len(g.data)-1 { // 最后一个元素无法进位
					g.data[i].Reset()
					s, _ := g.data[i].GenerateOne()
					g.container[i] = s
				}
			} else {
				g.container[i] = s
				isOk = true
				break
			}
			i++
		}
		return strings.Join(g.container, ""), isOk
	}
}

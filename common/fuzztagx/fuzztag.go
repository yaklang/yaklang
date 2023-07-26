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
}

// String
type StringNode struct {
	*NodeAttr
	index int
	data  string
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
	ok := true
	if s.index > 0 {
		ok = false
	} else {
		s.index++
	}
	return s.data, ok
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
			f.index = 0
		} else {
			return "", false
		}
	}

	if f.index >= len(*f.cache) {
		if !f.isRep {
			return "", false
		} else {
			return utils.GetLastElement(*f.cache), true
		}
	}
	defer func() {
		f.index++
	}()
	return (*f.cache)[f.index], true

}

type Generator struct {
	container []string
	index     int
	data      []Node
	first     bool
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
		for {
			if len(g.data) == g.index {
				break
			}
			s, ok := g.data[g.index].GenerateOne()
			if !ok {
				g.index++
			} else {
				g.container[g.index] = s
				isOk = true
				break
			}
		}
		return strings.Join(g.container, ""), isOk
	}
}

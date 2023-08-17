package fuzztagx

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"reflect"
	"strings"
	"unsafe"
)

type NodeAttr struct {
	index int
	isDyn bool
	isRep bool
}
type Node interface {
	Strings() []string
	GenerateOne() bool
	Reset()
}

// String
type StringNode struct {
	*NodeAttr
	backpropagation func(s string)
	data            string
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
func (s *StringNode) GenerateOne() bool {
	if s.index > 0 {
		s.backpropagation("")
		return false
	} else {
		s.index++
		s.backpropagation(s.data)
		return true
	}
}
func (s *StringNode) Reset() {
	s.index = 0
}

// Expression
type ExpressionNode struct {
	*NodeAttr
	backpropagation func(s string)
	data            string
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
func (f *ExpressionNode) GenerateOne() bool {
	if f.index > 0 {
		return false
	} else {
		box := httptpl.NewNucleiDSLYakSandbox()
		res, err := box.Execute(f.data)
		if err != nil {
			f.backpropagation("")
			return true
		} else {
			f.backpropagation(utils.InterfaceToString(res))
			return true
		}
	}
}
func (s *ExpressionNode) Reset() {
	s.index = 0
}

// FuzzTag/ExpressionTag
type Tag struct {
	IsExpTag        bool
	Nodes           []Node
	generator       *Generator
	backpropagation func(s string)
}

func (f *Tag) Strings() []string {
	return nil
}
func (f *Tag) GenerateOne() bool {
	if f.generator == nil {
		f.generator = NewBackpropagationGenerator(f.backpropagation, f.Nodes)
	}
	s, ok := f.generator.Generate()
	if !ok {
		return false
	}
	f.backpropagation(s)
	return true
}
func (s *Tag) Reset() {
	s.generator = nil
	for _, node := range s.Nodes {
		node.Reset()
	}
}

// FuzzTagMethod
type FuzzTagMethod struct {
	cache           *[]string
	name            string
	label           string
	isDyn           bool
	isRep           bool
	params          []Node
	methodCtx       *MethodContext
	generator       *Generator
	index           int
	backpropagation func(s string)
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

func (f *FuzzTagMethod) GenerateOne() bool {
	if f.generator == nil { // 未初始化
		f.generator = NewBackpropagationGenerator(f.backpropagation, f.params)
		f.ParseLabel()
		if f.label != "" {
			if v, ok := f.methodCtx.labelTable[f.label]; ok {
				f.methodCtx.labelTable[f.label] = append(v, f)
			} else {
				f.methodCtx.labelTable[f.label] = []*FuzzTagMethod{f}
			}
		}
	}
CHECK:
	if f.cache == nil || f.isDyn || f.index >= len(*f.cache) {
		if f.methodCtx.methodTable != nil {
			fun, ok := (f.methodCtx.methodTable)[f.name]
			if !ok {
				f.backpropagation("")
				return false
			}
			s, ok := f.generator.Generate()
			if !ok {
				f.backpropagation("")
				return false
			}
			result := fun(s)
			f.cache = &result
			if f.index >= len(*f.cache) {
				f.index = 0
			}
			goto CHECK
		} else {
			f.backpropagation("")
			return false
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
	f.backpropagation((*f.cache)[f.index])
	return true
}
func (s *FuzzTagMethod) Reset() {
	s.generator = nil
	s.index = 0
	s.cache = nil
	for _, param := range s.params {
		param.Reset()
	}
}

type LabelMethods struct {
	methods []*FuzzTagMethod
}

func (l *LabelMethods) Reset() {
	for _, method := range l.methods {
		method.Reset()
	}
}
func (l *LabelMethods) GenerateOne() bool {
	ok := false
	for _, method := range l.methods {
		genOk := method.GenerateOne()
		if genOk {
			ok = true
		}
	}
	return ok
}

type GeneratorContext struct {
}
type Generator struct {
	container []string
	//index     int
	data            []Node
	first           bool
	ctx             *GeneratorContext
	backpropagation func(s string)
}

func NewBackpropagationGenerator(f func(s string), nodes []Node) *Generator {
	g := &Generator{data: nodes, container: make([]string, len(nodes)), first: true, backpropagation: f}
	for index, d := range g.data {
		switch ret := d.(type) {
		case *Tag:
			ret.backpropagation = func(index int) func(s string) {
				return func(s string) {
					g.container[index] = s
					g.backpropagation(s)
				}
			}(index)
		case *FuzzTagMethod:
			ret.backpropagation = func(index int) func(s string) {
				return func(s string) {
					g.container[index] = s
					g.backpropagation(s)
				}
			}(index)
		case *StringNode:
			ret.backpropagation = func(index int) func(s string) {
				return func(s string) {
					g.container[index] = s
					g.backpropagation(s)
				}
			}(index)
		case *ExpressionNode:
			ret.backpropagation = func(index int) func(s string) {
				return func(s string) {
					g.container[index] = s
					g.backpropagation(s)
				}
			}(index)
		}
		d.GenerateOne()
	}
	return g
}
func NewGenerator(nodes []Node) *Generator {
	return NewBackpropagationGenerator(func(s string) {

	}, nodes)
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
		renderedNode := map[unsafe.Pointer]struct{}{}
		for {
			if len(g.data) == i {
				break
			}
			uid := reflect.ValueOf(g.data[i]).UnsafePointer()
			if _, ok := renderedNode[uid]; ok {
				i++
				continue
			}
			renderedNode[reflect.ValueOf(g.data[i]).UnsafePointer()] = struct{}{}
			ok := g.data[i].GenerateOne()

			if !ok {
				if i < len(g.data)-1 { // 最后一个元素无法进位
					g.data[i].Reset()
					g.data[i].GenerateOne()
				}
			} else {
				if v, ok := g.data[i].(*FuzzTagMethod); ok {
					if ms, ok := v.methodCtx.labelTable[v.label]; ok {
						for _, m := range ms {
							uid1 := reflect.ValueOf(m).UnsafePointer()
							if uid1 == uid {
								continue
							}
							renderedNode[uid1] = struct{}{}
							m.GenerateOne()
						}
					}
				}
				isOk = true
				break
			}
			i++
		}
		return strings.Join(g.container, ""), isOk
	}
}

package parser

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"reflect"
	"strings"
	"unsafe"
)

type GeneratorNode interface {
	Reset()
	GenerateOne() bool
}

type MethodContext struct {
	methodTable map[string]func(s string) []string
	labelTable  map[string][]*TagGenerator
}

// String
type StringGenerator struct {
	backpropagation func(s string)
	index           int
	data            string
}

func (s *StringGenerator) GenerateOne() bool {
	if s.index > 0 {
		s.backpropagation("")
		return false
	} else {
		s.index++
		s.backpropagation(s.data)
		return true
	}
}
func (s *StringGenerator) Reset() {
	s.index = 0
}

// Expression
type ExpressionGenerator struct {
	backpropagation func(s string)
	index           int
	data            *FuzzTag
}

func (f *ExpressionGenerator) GenerateOne() bool {
	if f.index > 0 {
		return false
	} else {
		box := httptpl.NewNucleiDSLYakSandbox()
		res, err := box.Execute(utils.InterfaceToString(f.data.Data[0])) // 可能越界
		if err != nil {
			f.backpropagation("")
			return true
		} else {
			f.backpropagation(utils.InterfaceToString(res))
			return true
		}
	}
}
func (s *ExpressionGenerator) Reset() {
	s.index = 0
}

// FuzzTag
type TagGenerator struct {
	data            *FuzzTag
	cache           *[]string
	isDyn           bool
	isRep           bool
	params          []GeneratorNode
	methodCtx       *MethodContext
	generator       *Generator
	index           int
	backpropagation func(s string)
}

func NewTagGenerator(tag *FuzzTag, ctx *MethodContext) *TagGenerator {
	return &TagGenerator{
		data:      tag,
		methodCtx: ctx,
	}
}

func (f *TagGenerator) GenerateOne() bool {
	if f.generator == nil { // 未初始化
		f.generator = NewBackpropagationGenerator(f.backpropagation, f.params)
	}
CHECK:
	if f.cache == nil || f.isDyn || f.index >= len(*f.cache) {
		if f.methodCtx.methodTable != nil {
			fun, ok := (f.methodCtx.methodTable)[f.data.Method]
			if !ok {
				f.backpropagation("")
				return false
			}
			s, ok := f.generator.Generate()
			if !ok {
				f.backpropagation("")
				return false
			}
			if f.cache != nil && f.index >= len(*f.cache) {
				f.index = 0
			}
			result := fun(s)
			f.cache = &result

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
func (s *TagGenerator) Reset() {
	s.generator = nil
	s.index = 0
	s.cache = nil
	for _, param := range s.params {
		param.Reset()
	}
}

type LabelFuzzTagGenerator struct {
	methods []*TagGenerator
}

func (l *LabelFuzzTagGenerator) Reset() {
	for _, method := range l.methods {
		method.Reset()
	}
}
func (l *LabelFuzzTagGenerator) GenerateOne() bool {
	ok := false
	for _, method := range l.methods {
		genOk := method.GenerateOne()
		if genOk {
			ok = true
		}
	}
	return ok
}

type GeneratorConfig struct {
	AllDynMethod   bool
	DynMethodNames *utils.Set
	MethodTable    map[string]func(string) []string
}

type Generator struct {
	container []string
	//index     int
	data            []GeneratorNode
	first           bool
	backpropagation func(s string)
}

func NewBackpropagationGenerator(f func(s string), nodes []GeneratorNode) *Generator {
	g := &Generator{data: nodes, container: make([]string, len(nodes)), first: true, backpropagation: f}
	for index, d := range g.data {
		switch ret := d.(type) {
		case *TagGenerator:
			ret.backpropagation = func(index int) func(s string) {
				return func(s string) {
					g.container[index] = s
					g.backpropagation(s)
				}
			}(index)
		case *StringGenerator:
			ret.backpropagation = func(index int) func(s string) {
				return func(s string) {
					g.container[index] = s
					g.backpropagation(s)
				}
			}(index)
		case *ExpressionGenerator:
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
func NewGeneratorWithConfig(nodes []Node, config *GeneratorConfig) *Generator {
	labelTable := map[string][]*TagGenerator{}
	var node2generator func(nodes []Node) []GeneratorNode
	node2generator = func(nodes []Node) []GeneratorNode {
		methodCtx := &MethodContext{
			methodTable: config.MethodTable,
			labelTable:  labelTable,
		}
		generatorNodes := []GeneratorNode{}
		for _, node := range nodes {
			switch ret := node.(type) {
			case *FuzzTag:
				gener := NewTagGenerator(ret, methodCtx)
				gener.params = node2generator(ret.Data)
				generatorNodes = append(generatorNodes, gener)
				if ret.Label != "" {
					labelTable[ret.Label] = append(labelTable[ret.Label], gener)
				}
			case string:
				generatorNodes = append(generatorNodes, &StringGenerator{data: ret})
			}
		}
		return generatorNodes
	}
	return NewBackpropagationGenerator(func(s string) {}, node2generator(nodes))
}
func NewGenerator(nodes []Node, funcMap map[string]func(string) []string) *Generator {
	return NewGeneratorWithConfig(nodes, &GeneratorConfig{
		MethodTable:  funcMap,
		AllDynMethod: true,
	})
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
			genOneOk := g.data[i].GenerateOne()
			if v, ok := g.data[i].(*TagGenerator); ok {
				if ms, ok := v.methodCtx.labelTable[v.data.Label]; ok {
					for _, m := range ms {
						uid1 := reflect.ValueOf(m).UnsafePointer()
						if uid1 == uid {
							continue
						}
						renderedNode[uid1] = struct{}{}
						if m.GenerateOne() {
							genOneOk = true
						}
					}
				}
			}
			if !genOneOk {
				if i < len(g.data)-1 { // 最后一个元素无法进位
					g.data[i].Reset()
					g.data[i].GenerateOne()
				}
			} else {
				isOk = true
				break
			}
			i++
		}
		return strings.Join(g.container, ""), isOk
	}
}

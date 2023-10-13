package standard_parser

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"reflect"
	"unsafe"
)

type GeneratorNode interface {
	Reset()
	GenerateOne() bool
	IsRep() bool
}

type MethodContext struct {
	methodTable    map[string]TagMethod
	labelTable     map[string]map[*TagGenerator]struct{}
	tagToLabelsMap map[*TagGenerator][]string
}

// UpdateLabels 更新全局labelTable，先删除当前tag的所有label映射，再增加
func (m *MethodContext) UpdateLabels(tag *TagGenerator) {
	for _, label := range m.tagToLabelsMap[tag] {
		if set, ok := m.labelTable[label]; ok {
			delete(set, tag)
		}
	}
	m.tagToLabelsMap[tag] = tag.data.GetLabels()
	for _, label := range tag.data.GetLabels() {
		if _, ok := m.labelTable[label]; !ok {
			m.labelTable[label] = map[*TagGenerator]struct{}{}
		}
		m.labelTable[label][tag] = struct{}{}
	}
}

// String
type StringGenerator struct {
	backpropagation func(s FuzzResult)
	data            string
}

func (s *StringGenerator) GenerateOne() bool {
	s.backpropagation(FuzzResult(s.data))
	return false
}
func (s *StringGenerator) Reset() {
}
func (s *StringGenerator) IsRep() bool {
	return true
}

// Expression
type ExpressionGenerator struct {
	backpropagation func(s FuzzResult)
	index           int
	data            TagNode
	isRep           bool
	cache           string
}

func (f *ExpressionGenerator) GenerateOne() bool {
	if f.index > 0 {
		if f.isRep {
			f.backpropagation(FuzzResult(f.cache))
			return false
		}
		f.backpropagation(FuzzResult(""))
		return false
	} else {
		box := httptpl.NewNucleiDSLYakSandbox()
		res, err := box.Execute(utils.InterfaceToString(f.data.GetData()[0])) // 可能越界
		if err != nil {
			f.backpropagation(FuzzResult{})
			return false
		} else {
			f.backpropagation(FuzzResult(utils.InterfaceToString(res)))
			return true
		}
	}
}
func (s *ExpressionGenerator) Reset() {
	s.index = 0
}
func (s *ExpressionGenerator) IsRep() bool {
	return s.isRep
}

type TagGenerator struct {
	data            TagNode
	cache           *[]FuzzResult
	isRep           bool
	params          []GeneratorNode
	methodCtx       *MethodContext
	generator       *Generator
	index           int
	backpropagation func(s FuzzResult)
}

func NewTagGenerator(tag TagNode, ctx *MethodContext) *TagGenerator {
	return &TagGenerator{
		data:      tag,
		methodCtx: ctx,
	}
}
func (f *TagGenerator) Exec(data string) ([]FuzzResult, error) {
	res, err := f.data.Exec(data, f.methodCtx.methodTable)
	if err != nil {
		return nil, err
	}
	f.methodCtx.UpdateLabels(f)
	return res, nil
}
func (f *TagGenerator) GenerateOne() bool {
	if f.generator == nil { // 未初始化
		f.generator = NewBackpropagationGenerator(f.backpropagation, f.params)
	}
	if f.cache == nil {
		s, ok := f.generator.Generate()
		if ok {
			result, err := f.Exec(s)
			if err != nil {
				f.backpropagation([]byte(""))
				return false
			}
			f.cache = &result
		}
	}

	if f.cache == nil || len(*f.cache) == 0 {
		f.backpropagation([]byte(""))
		return false
	}
	if f.index >= len(*f.cache) {
		if f.isRep {
			f.backpropagation(utils.GetLastElement(*f.cache))
			return false
		}
		s, ok := f.generator.Generate()
		if !ok {
			f.backpropagation(FuzzResult(""))
			return false
		}
		result, err := f.Exec(s)
		if err != nil {
			f.backpropagation(FuzzResult(""))
			return false
		}
		f.cache = &result
		f.index = 0
	}
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
func (s *TagGenerator) IsRep() bool {
	return s.isRep
}

type Generator struct {
	container []FuzzResult
	//index     int
	data            []GeneratorNode
	first           bool
	backpropagation func(s FuzzResult)
}

func NewBackpropagationGenerator(f func(s FuzzResult), nodes []GeneratorNode) *Generator {
	g := &Generator{data: nodes, container: make([]FuzzResult, len(nodes)), first: true, backpropagation: f}
	for index, d := range g.data {
		switch ret := d.(type) {
		case *TagGenerator:
			ret.backpropagation = func(index int) func(s FuzzResult) {
				return func(s FuzzResult) {
					g.container[index] = s
					g.backpropagation(s)
				}
			}(index)
		case *StringGenerator:
			ret.backpropagation = func(index int) func(s FuzzResult) {
				return func(s FuzzResult) {
					g.container[index] = s
					g.backpropagation(s)
				}
			}(index)
		case *ExpressionGenerator:
			ret.backpropagation = func(index int) func(s FuzzResult) {
				return func(s FuzzResult) {
					g.container[index] = s
					g.backpropagation(s)
				}
			}(index)
		}
		d.GenerateOne()
	}
	return g
}
func NewGenerator(nodes []Node, table map[string]TagMethod) (*Generator, error) {
	methodCtx := &MethodContext{
		methodTable:    table,
		labelTable:     map[string]map[*TagGenerator]struct{}{},
		tagToLabelsMap: map[*TagGenerator][]string{},
	}
	var node2generator func(nodes []Node) []GeneratorNode
	node2generator = func(nodes []Node) []GeneratorNode {
		generatorNodes := []GeneratorNode{}
		for _, node := range nodes {
			switch ret := node.(type) {
			case TagNode:
				gener := NewTagGenerator(ret, methodCtx)
				gener.params = node2generator(ret.GetData())
				methodCtx.tagToLabelsMap[gener] = ret.GetLabels()
				generatorNodes = append(generatorNodes, gener)
				for _, label := range ret.GetLabels() {
					switch label {
					case "rep":
						gener.isRep = true
					default:
						methodCtx.labelTable[label][gener] = struct{}{}
					}
				}

			case StringNode:
				generatorNodes = append(generatorNodes, &StringGenerator{data: string(ret)})
			}
		}
		return generatorNodes
	}
	return NewBackpropagationGenerator(func(s FuzzResult) {}, node2generator(nodes)), nil
}

func (g *Generator) Generate() (string, bool) {
	if g.first {
		defer func() {
			g.first = false
		}()
		res := ""
		for _, result := range g.container {
			res += string(result)
		}
		return res, true
	} else {
		isOk := false
		i := 0
		renderedNode := map[unsafe.Pointer]struct{}{}
		successCallBacks := []func(){}
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
				for _, label := range v.data.GetLabels() {
					if ms, ok := v.methodCtx.labelTable[label]; ok {
						for m := range ms {
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
			}
			if !genOneOk {
				if !g.data[i].IsRep() && i < len(g.data)-1 { // 最后一个元素无法进位
					i := i
					successCallBacks = append(successCallBacks, func() {
						g.data[i].Reset()
						g.data[i].GenerateOne()
					})
				}
			} else {
				for _, back := range successCallBacks {
					back()
				}
				isOk = true
				break
			}
			i++
		}
		res := ""
		for _, result := range g.container {
			res += string(result)
		}
		return res, isOk
	}
}

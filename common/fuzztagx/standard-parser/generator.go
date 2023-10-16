package standard_parser

import (
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
	"unsafe"
)

type ExecNode interface {
	Reset()
	Exec() (bool, error)
	IsRep() bool
}

type MethodContext struct {
	methodTable    map[string]TagMethod
	labelTable     map[string]map[*TagExecNode]struct{}
	tagToLabelsMap map[*TagExecNode][]string
}

// UpdateLabels 更新全局labelTable，先删除当前tag的所有label映射，再增加
func (m *MethodContext) UpdateLabels(tag *TagExecNode) {
	for _, label := range m.tagToLabelsMap[tag] {
		if set, ok := m.labelTable[label]; ok {
			delete(set, tag)
		}
	}
	m.tagToLabelsMap[tag] = tag.data.GetLabels()
	for _, label := range tag.data.GetLabels() {
		if _, ok := m.labelTable[label]; !ok {
			m.labelTable[label] = map[*TagExecNode]struct{}{}
		}
		m.labelTable[label][tag] = struct{}{}
	}
}

// String
type StringExecNode struct {
	submitResult func(s FuzzResult)
	data         string
	index        int
}

func (s *StringExecNode) Exec() (bool, error) {
	if s.index == 0 {
		s.submitResult(FuzzResult(s.data))
		return false, nil
	}
	s.index++
	return false, nil
}
func (s *StringExecNode) Reset() {
	s.index = 0
}
func (s *StringExecNode) IsRep() bool {
	return true
}

type TagExecNode struct {
	data         TagNode
	cache        *[]FuzzResult
	isRep        bool
	params       []ExecNode
	methodCtx    *MethodContext
	generator    *Generator
	index        int
	submitResult func(s FuzzResult)
}

func NewTagGenerator(tag TagNode, ctx *MethodContext) *TagExecNode {
	return &TagExecNode{
		data:      tag,
		methodCtx: ctx,
	}
}

func (f *TagExecNode) Exec() (bool, error) {
	if f.index >= len(*f.cache) {
		if f.isRep { // 等价于StringExecNode
			f.submitResult(utils.GetLastElement(*f.cache))
			return false, nil
		} else {
			return false, nil
		}
	}
	defer func() {
		f.index++
	}()
	f.submitResult((*f.cache)[f.index])
	return true, nil
}
func (s *TagExecNode) Reset() {
	s.index = 0
	//s.cache = nil
	//for _, param := range s.params {
	//	param.Reset()
	//}
}
func (s *TagExecNode) IsRep() bool {
	return s.isRep
}

type Generator struct {
	container []FuzzResult
	//index     int
	data            []ExecNode
	first           bool
	backpropagation func(s FuzzResult) error
}

func NewBackpropagationGenerator(f func(s FuzzResult) error, nodes []ExecNode) (*Generator, error) {
	g := &Generator{data: nodes, container: make([]FuzzResult, len(nodes)), first: true, backpropagation: f}
	for index, d := range g.data {
		index := index
		switch ret := d.(type) {
		case *TagExecNode:
			ret.submitResult = func(s FuzzResult) {
				g.container[index] = s
			}
			var bp func(s FuzzResult) error
			bpP := &bp
			childGen, err := NewBackpropagationGenerator(*bpP, ret.params)
			if err != nil {
				return nil, err
			}
			*bpP = func(s FuzzResult) error {
				s, _, err := childGen.Generate()
				res, err := ret.data.Exec(s, ret.methodCtx.methodTable)
				if err != nil {
					return err
				}
				ret.methodCtx.UpdateLabels(ret)
				if len(res) == 0 {
					res = []FuzzResult{FuzzResult("")}
				}
				ret.cache = &res
				ret.index = 1
				ret.submitResult((*ret.cache)[0])
				return nil
			}
		case *StringExecNode:
			ret.submitResult = func(s FuzzResult) {
				g.container[index] = s
			}
		}
	}
	return g, nil
}
func NewGenerator(nodes []Node, table map[string]TagMethod) (*Generator, error) {
	methodCtx := &MethodContext{
		methodTable:    table,
		labelTable:     map[string]map[*TagExecNode]struct{}{},
		tagToLabelsMap: map[*TagExecNode][]string{},
	}
	var node2generator func(nodes []Node) []ExecNode
	node2generator = func(nodes []Node) []ExecNode {
		generatorNodes := []ExecNode{}
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
				generatorNodes = append(generatorNodes, &StringExecNode{data: string(ret)})
			}
		}
		return generatorNodes
	}
	return NewBackpropagationGenerator(func(s FuzzResult) error { return nil }, node2generator(nodes))
}

func (g *Generator) ContactContainers() FuzzResult {
	res := FuzzResult("")
	for _, result := range g.container {
		res = append(res, result...)
	}
	return res
}
func (g *Generator) Generate() (FuzzResult, bool, error) {
	isOk := false
	i := 0
	renderedNode := map[unsafe.Pointer]struct{}{}
	successCallBacks := []func() error{}
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
		genOneOk, err := g.data[i].Exec()
		if !g.first {
			err := g.backpropagation(g.ContactContainers())
			if err != nil {
				return nil, false, err
			}
		} else {
			g.first = false
		}
		if err != nil {
			return nil, false, err
		}
		if v, ok := g.data[i].(*TagExecNode); ok {
			for _, label := range v.data.GetLabels() {
				if ms, ok := v.methodCtx.labelTable[label]; ok {
					for m := range ms {
						uid1 := reflect.ValueOf(m).UnsafePointer()
						if uid1 == uid {
							continue
						}
						renderedNode[uid1] = struct{}{}
						ok1, err := m.Exec()
						if err != nil {
							return nil, false, err
						}
						genOneOk = ok1
					}
				}
			}
		}
		if !genOneOk {
			if !g.data[i].IsRep() && i < len(g.data)-1 { // 最后一个元素无法进位
				i := i
				successCallBacks = append(successCallBacks, func() error {
					g.data[i].Reset()
					_, err := g.data[i].Exec()
					return err
				})
			}
		} else {
			for _, back := range successCallBacks {
				if err := back(); err != nil {
					return nil, false, err
				}
			}
			isOk = true
			break
		}
		i++
	}
	res := FuzzResult("")
	for _, result := range g.container {
		res = append(res, result...)
	}
	return res, isOk, nil

}

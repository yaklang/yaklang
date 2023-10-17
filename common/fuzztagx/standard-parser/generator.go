package standard_parser

import (
	"reflect"
	"unsafe"
)

type ExecNode interface {
	Reset()              // 重置生成器
	Exec() (bool, error) // 优先读取缓存，读完缓存后调用子生成器
	FirstExec() error    // 第一次执行不进行反向传播
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
		switch label {
		case "rep":
			tag.isRep = true
		case "dyn":
			tag.isDyn = true
		default:
			if _, ok := m.labelTable[label]; !ok {
				m.labelTable[label] = map[*TagExecNode]struct{}{}
			}
			m.labelTable[label][tag] = struct{}{}
		}
	}
}

// String
type StringExecNode struct {
	submitResult func(s FuzzResult)
	data         string
	index        int
}

func (s *StringExecNode) FirstExec() error {
	s.index = 1
	s.submitResult(FuzzResult(s.data))
	return nil
}
func (s *StringExecNode) Exec() (bool, error) {
	if s.index == 0 {
		s.submitResult(FuzzResult(s.data))
		return true, nil
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
	data            TagNode
	cache           *[]FuzzResult
	isRep           bool
	isDyn           bool
	params          []ExecNode
	methodCtx       *MethodContext
	childGenerator  *Generator //
	index           int
	submitResult    func(s FuzzResult)
	backpropagation func() error
}

func NewTagGenerator(tag TagNode, ctx *MethodContext) *TagExecNode {
	return &TagExecNode{
		data:      tag,
		methodCtx: ctx,
	}
}

// FirstExec 重置并执行
func (f *TagExecNode) FirstExec() error {
	_, err := f.childGenerator.Generate()
	if err != nil {
		return err
	}
	return f.exec(f.childGenerator.Result())
}
func (f *TagExecNode) exec(s FuzzResult) error {
	res, err := f.data.Exec(s, f.methodCtx.methodTable)
	if err != nil {
		return err
	}
	f.methodCtx.UpdateLabels(f)
	if len(res) == 0 {
		res = []FuzzResult{FuzzResult("")}
	}
	f.cache = &res
	f.index = 1
	f.submitResult((*f.cache)[0])
	return nil
}
func (f *TagExecNode) Exec() (bool, error) {
	if f.index >= len(*f.cache) {
		ok, err := f.childGenerator.Generate()
		if err != nil {
			return false, err
		}
		if f.isRep { // 当生成失败且存在rep标签时，使用最后一个元素
			f.submitResult((*f.cache)[len(*f.cache)-1])
			return false, f.backpropagation()
		}
		if !ok {
			return false, nil
		}
		return true, f.backpropagation()
	}
	defer func() {
		f.index++
	}()
	f.submitResult((*f.cache)[f.index])
	return true, f.backpropagation()
}
func (s *TagExecNode) Reset() {
	s.index = 0
}
func (s *TagExecNode) IsRep() bool {
	return s.isRep
}

type Generator struct {
	container []FuzzResult
	//index     int
	data            []ExecNode
	first           bool
	backpropagation func() error
}

func newBackpropagationGenerator(f func() error, nodes []ExecNode) *Generator {
	g := &Generator{data: nodes, container: make([]FuzzResult, len(nodes)), first: true, backpropagation: f}
	for index, d := range g.data {
		index := index
		switch ret := d.(type) {
		case *TagExecNode:
			ret.submitResult = func(s FuzzResult) {
				g.container[index] = s
			}
			ret.backpropagation = f
			var bp func() error
			childGen := newBackpropagationGenerator(func() error {
				return bp()
			}, ret.params)
			bp = func() error {
				return ret.exec(childGen.Result())
			}
			ret.childGenerator = childGen
		case *StringExecNode:
			ret.submitResult = func(s FuzzResult) {
				g.container[index] = s
			}
		}
	}
	return g
}
func NewGenerator(nodes []Node, table map[string]TagMethod) *Generator {
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
					methodCtx.labelTable[label][gener] = struct{}{}
				}
			case StringNode:
				generatorNodes = append(generatorNodes, &StringExecNode{data: string(ret)})
			}
		}
		return generatorNodes
	}
	g := newBackpropagationGenerator(func() error {
		return nil
	}, node2generator(nodes))
	return g
}

func (g *Generator) Result() FuzzResult {
	res := FuzzResult("")
	for _, result := range g.container {
		res = append(res, result...)
	}
	return res
}
func (g *Generator) Generate() (bool, error) {
	if g.first {
		for _, d := range g.data {
			err := d.FirstExec()
			if err != nil {
				return false, err
			}
		}
		g.first = false
		return true, nil
	}
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
		if err != nil {
			return false, err
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
							return false, err
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
					if v, ok := g.data[i].(*TagExecNode); ok {
						if v.isDyn { //重新执行当前和所有子节点
							var execAllFirst func(t ExecNode)
							execAllFirst = func(t ExecNode) {
								if v1, ok := t.(*TagExecNode); ok {
									for _, param := range v1.params {
										execAllFirst(param)
									}
									t.FirstExec() //在这个节点第一次执行时已经判断了err，这里不用判断了
								}
							}
							execAllFirst(v)
							return v.backpropagation()
						}
					}
					g.data[i].Reset()
					_, err := g.data[i].Exec()
					return err
				})
			}
		} else {
			for _, back := range successCallBacks {
				if err := back(); err != nil {
					return false, err
				}
			}
			isOk = true
			break
		}
		i++
	}
	return isOk, nil
}

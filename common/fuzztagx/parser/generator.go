package parser

import (
	"reflect"
	"unsafe"
)

type GenerateConfig struct {
	AssertError bool
}
type ExecNode interface {
	Reset()              // 重置生成器
	Exec() (bool, error) // 优先读取缓存，读完缓存后调用子生成器
	IsRep() bool
}

type MethodContext struct {
	methodTable    map[string]*TagMethod
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
	submitResult func(s *FuzzResult)
	data         string
	index        int
}

func (s *StringExecNode) FirstExec() error {
	s.index = 1
	s.submitResult(NewFuzzResultWithData(s.data))
	return nil
}
func (s *StringExecNode) Exec() (bool, error) {
	defer func() {
		s.index++
	}()
	if s.index == 0 {
		s.submitResult(NewFuzzResultWithData(s.data))
		return false, nil
	}
	return false, nil
}

func (s *StringExecNode) Reset() {
	s.index = 0
}
func (s *StringExecNode) IsRep() bool {
	return true
}

type TagExecNode struct {
	config          *GenerateConfig
	data            TagNode
	cache           *[]*FuzzResult
	isRep           bool
	isDyn           bool
	params          []ExecNode
	methodCtx       *MethodContext
	childGenerator  *Generator //
	index           int
	submitResult    func(s *FuzzResult)
	backpropagation func() error
	ignoreError     bool
}

func NewTagGenerator(tag TagNode, ctx *MethodContext) *TagExecNode {
	return &TagExecNode{
		data:      tag,
		methodCtx: ctx,
	}
}

// FirstExec 重置并执行
func (f *TagExecNode) FirstExec(bp, exec, all bool) error {
	f.childGenerator.first = exec
	_, err := f.childGenerator.generate()
	if err != nil {
		return err
	}

	if exec {
		err = f.exec(f.childGenerator.Result())
		if err != nil {
			return err
		}
	}
	f.index = 1
	f.submitResult((*f.cache)[0])
	if all {
		for _, param := range f.params {
			if v, ok := param.(*TagExecNode); ok {
				v.FirstExec(bp, exec, all)
			}
		}
	}
	return err
}
func (f *TagExecNode) exec(s *FuzzResult) error {
	res, err := f.data.Exec(s, f.methodCtx.methodTable)
	if len(res) == 0 {
		res = []*FuzzResult{NewFuzzResultWithData("")}
	}
	for _, r := range res {
		r.Source = append(r.Source, s)
		r.ByTag = true
		r.Error = err
	}

	if !f.config.AssertError {
		err = nil
	}
	if err != nil {
		return err
	}
	f.methodCtx.UpdateLabels(f)
	f.cache = &res
	return nil
}
func (f *TagExecNode) Exec() (bool, error) {
	defer func() {
		f.index++
	}()
	if f.index >= len(*f.cache) {
		if f.isRep { // 当生成失败且存在rep标签时，使用最后一个元素
			f.submitResult((*f.cache)[len(*f.cache)-1])
			return false, f.backpropagation()
		}
		ok, err := f.childGenerator.generate()
		if err != nil {
			return ok, err
		}
		if !ok {
			f.submitResult(NewFuzzResultWithData(""))
			return false, nil
		} else {
			return true, f.backpropagation()
		}
	}
	f.submitResult((*f.cache)[f.index])
	return true, f.backpropagation()
}
func (s *TagExecNode) Reset() {
	s.index = 0
	var bp func() error
	for _, param := range s.params {
		param.Reset()
		v, ok := param.(*TagExecNode)
		if ok {
			v.submitResult((*v.cache)[v.index])
			bp = v.backpropagation
		}
	}
	if bp != nil {
		bp()
	}
}
func (s *TagExecNode) IsRep() bool {
	return s.isRep
}

type Generator struct {
	*GenerateConfig
	container []*FuzzResult
	//index     int
	data            []ExecNode
	first           bool
	backpropagation func() error
	AssertError     bool
	Error           error
	allowedLabels   bool
}

func newBackpropagationGenerator(f func() error, nodes []ExecNode, cfg *GenerateConfig) *Generator {
	g := &Generator{data: nodes, container: make([]*FuzzResult, len(nodes)), first: true, backpropagation: f, GenerateConfig: cfg}
	for index, d := range g.data {
		index := index
		switch ret := d.(type) {
		case *TagExecNode:
			ret.config = cfg
			ret.submitResult = func(s *FuzzResult) {
				g.container[index] = s
			}
			ret.backpropagation = f
			var bp func() error
			childGen := newBackpropagationGenerator(func() error {
				return bp()
			}, ret.params, cfg)
			bp = func() error {
				err := ret.exec(childGen.Result())
				ret.index = 0
				ret.submitResult((*ret.cache)[0])
				return err
			}
			ret.childGenerator = childGen
		case *StringExecNode:
			ret.submitResult = func(s *FuzzResult) {
				g.container[index] = s
			}
		}
	}
	return g
}
func NewGenerator(nodes []Node, table map[string]*TagMethod) *Generator {
	cfg := &GenerateConfig{}
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
	}, node2generator(nodes), cfg)
	g.allowedLabels = true
	return g
}

func (g *Generator) Result() *FuzzResult {
	res := NewFuzzResult()
	data := []byte{}
	for _, result := range g.container {
		data = append(data, result.GetData()...)
		res.Source = append(res.Source, result)
	}
	res.Data = data
	res.Contact = true
	return res
}
func (g *Generator) Next() bool {
	ok, err := g.generate()
	g.Error = err
	return ok
}
func (g *Generator) generate() (bool, error) {
	if g.first {
		for _, d := range g.data {
			switch ret := d.(type) {
			case *TagExecNode:
				err := ret.FirstExec(false, true, false)
				if err != nil {
					return true, err
				}
			case *StringExecNode:
				ret.submitResult(NewFuzzResultWithData(ret.data))
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
			return genOneOk, err
		}
		if v, ok := g.data[i].(*TagExecNode); ok && g.allowedLabels {
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
							return ok1, err
						}
						genOneOk = genOneOk || ok1
					}
				}
			}
		}
		if !genOneOk {
			if i < len(g.data)-1 { // 最后一个元素无法进位
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
									v1.FirstExec(false, true, true) //在这个节点第一次执行时已经判断了err，这里不用判断了
								}
							}
							execAllFirst(v)
							return v.backpropagation()
						}
						return v.FirstExec(true, false, true)
					}
					return nil
				})
			}
		} else {
			for _, back := range successCallBacks {
				if err := back(); err != nil {
					return true, err
				}
			}
			isOk = true
			break
		}
		i++
	}
	return isOk, nil
}

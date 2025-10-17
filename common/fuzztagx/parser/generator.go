package parser

import (
	"context"
	"reflect"
	"sync"
	"unsafe"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type GenerateConfig struct {
	ctx         context.Context
	swg         *utils.SizedWaitGroup
	cancelCtx   func()
	AssertError bool
	logger      *log.Logger
}

func (g *GenerateConfig) Debug() {
	g.logger.SetLevel("debug")
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
	dynTag         map[*TagExecNode]struct{}
	globalTagNode  []*TagExecNode
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
			m.dynTag[tag] = struct{}{}
		case "flowcontrol":
			tag.isFlowControl = true
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
	cache           []*FuzzResult
	getter          func() (*FuzzResult, error)
	finished        bool
	isRep           bool
	isDyn           bool
	isFlowControl   bool
	params          []ExecNode
	methodCtx       *MethodContext
	childGenerator  *Generator //
	index           int
	submitResult    func(s *FuzzResult)
	backpropagation func() error
	ignoreError     bool
	parentNode      ExecNode
}

func NewTagGenerator(tag TagNode, ctx *MethodContext) *TagExecNode {
	return &TagExecNode{
		data:      tag,
		methodCtx: ctx,
	}
}

func (f *TagExecNode) GetCache(index int) (*FuzzResult, bool, error) {
	var err error
	var data *FuzzResult
	for index >= len(f.cache) {
		data, err = f.getter()
		if data == nil {
			if err != nil {
				if f.config.AssertError {
					return NewFuzzResultWithData(""), false, err
				} else {
					resData := NewFuzzResultWithData("")
					resData.ByTag = true
					resData.Error = err
					return resData, false, nil
				}
			} else {
				resData := NewFuzzResultWithData("")
				resData.ByTag = true
				resData.Error = err
				return resData, false, err
			}
		} else {
			f.cache = append(f.cache, data)
		}
	}
	return (f.cache)[index], true, err
}
func (f *TagExecNode) FirstExecWithBackpropagation(bp, exec, all bool) error {
	f.FirstExec(bp, exec, all)
	return f.backpropagation()
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
	res, _, err := f.GetCache(0)
	f.submitResult(res)
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
	defer func() {
		f.cache = nil
	}()
	ch := make(chan *FuzzResult)
	execCtx, execCtxCancel := context.WithCancel(f.config.ctx)
	var execEnd bool
	receiverLock := sync.Mutex{}
	receiver := func(result *FuzzResult) {
		receiverLock.Lock()
		defer receiverLock.Unlock()
		f.config.logger.Debugf("tag %s generate data: %s", f.data.String(), string(result.GetData()))
		if execEnd {
			return
		}
		select {
		case <-f.config.ctx.Done():
			execEnd = true
		case <-execCtx.Done():
			execEnd = true
		case ch <- result:
		}
	}
	var err error
	getter := func() (*FuzzResult, error) {
		return <-ch, err
	}

	go func() {
		f.config.swg.Add(1)
		defer f.config.swg.Done()
		defer func() {
			if e := recover(); e != nil {
				err = utils.Error(e)
			}
			close(ch)
			execCtxCancel()
		}()
		if f.data.IsFlowControl() {
			err = f.data.Exec(execCtx, s, func(result *FuzzResult) {}, f.methodCtx.methodTable)
		} else {
			err = f.data.Exec(execCtx, s, receiver, f.methodCtx.methodTable)
		}
	}()

	newGetter := func() (*FuzzResult, error) {
		r, err := getter()
		if r == nil {
			f.finished = true
			return nil, err
		}
		f.methodCtx.UpdateLabels(f)
		r.Source = append(r.Source, s)
		r.ByTag = true
		r.Error = err
		return r, err
	}
	f.getter = newGetter
	//if len(res) == 0 {
	//	res = []*FuzzResult{NewFuzzResultWithData("")}
	//}
	//for _, r := range res {
	//
	//}

	if !f.config.AssertError {
		err = nil
	}
	if err != nil {
		return err
	}
	//f.methodCtx.UpdateLabels(f)
	//f.cache = &res
	return nil
}
func (f *TagExecNode) Exec() (bool, error) {
	if f.isDyn && f.index >= 1 {
		f.finished = true
		f.cache = nil
		return false, nil
	}
	defer func() {
		f.index++
	}()
	data, ok, _ := f.GetCache(f.index)
	if !ok {
		if f.isRep { // 当生成失败且存在rep标签时，使用最后一个元素
			data1, _, _ := f.GetCache(f.index - 1)
			f.submitResult(data1)
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
			f.index = 0
			return true, f.backpropagation()
		}
	}
	f.submitResult(data)
	return true, f.backpropagation()
}
func (s *TagExecNode) Reset() {
	s.index = 0
	s.finished = false
	var bp func() error
	for _, param := range s.params {
		param.Reset()
		v, ok := param.(*TagExecNode)
		if ok {
			data, _, _ := s.GetCache(v.index)
			v.submitResult(data)
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
	data                   []ExecNode
	first                  bool
	methodCtx              *MethodContext
	backpropagation        func() error
	AssertError            bool
	Error                  error
	allowedLabels          bool
	renderTagWithSyncIndex bool
}

func newBackpropagationGenerator(f func() error, nodes []ExecNode, cfg *GenerateConfig) *Generator {
	g := &Generator{data: nodes, container: make([]*FuzzResult, len(nodes)), first: true, backpropagation: f, GenerateConfig: cfg}
	for index, d := range g.data {
		index := index
		switch ret := d.(type) {
		case *TagExecNode:
			g.methodCtx = ret.methodCtx
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
				ret.index = 1
				data, _, _ := ret.GetCache(0)
				ret.submitResult(data)
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
func NewGenerator(ctx context.Context, nodes []Node, table map[string]*TagMethod) *Generator {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	cfg := &GenerateConfig{
		ctx:       ctx,
		swg:       utils.NewSizedWaitGroup(-1), // 限制并发100
		cancelCtx: cancel,
		logger:    log.GetLogger("fuzztag"),
	}
	methodCtx := &MethodContext{
		dynTag:         map[*TagExecNode]struct{}{},
		methodTable:    table,
		labelTable:     map[string]map[*TagExecNode]struct{}{},
		tagToLabelsMap: map[*TagExecNode][]string{},
	}
	var globalTagNodeList []*TagExecNode
	var node2generator func(nodes []Node, parentNode ExecNode, deep int) []ExecNode
	node2generator = func(nodes []Node, parentNode ExecNode, deep int) []ExecNode {
		generatorNodes := []ExecNode{}
		for _, node := range nodes {
			switch ret := node.(type) {
			case TagNode:
				gener := NewTagGenerator(ret, methodCtx)
				gener.params = node2generator(ret.GetData(), gener, deep+1)
				gener.parentNode = parentNode
				methodCtx.tagToLabelsMap[gener] = ret.GetLabels()
				generatorNodes = append(generatorNodes, gener)
				methodCtx.UpdateLabels(gener)
				if deep == 0 {
					globalTagNodeList = append(globalTagNodeList, gener)
				}
				//for _, label := range ret.GetLabels() {
				//	methodCtx.labelTable[label][gener] = struct{}{}
				//}
			case StringNode:
				generatorNodes = append(generatorNodes, &StringExecNode{data: string(ret)})
			}
		}
		return generatorNodes
	}
	g := newBackpropagationGenerator(func() error {
		return nil
	}, node2generator(nodes, nil, 0), cfg)
	g.allowedLabels = true
	if g.methodCtx != nil {
		g.methodCtx.globalTagNode = globalTagNodeList
	}
	return g
}

func (g *Generator) Wait() {
	g.swg.Wait()
}

func (g *Generator) Cancel() {
	if g.cancelCtx != nil {
		g.cancelCtx()
	}
}

func (g *Generator) RawResult() []*FuzzResult {
	return g.container
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
	defer func() {
		if e := recover(); e != nil {
			if err, ok := e.(error); ok {
				g.Error = err
			} else {
				g.Error = utils.Error(e)
			}
		}
	}()
	ok, err := g.generate()
	g.Error = err
	return ok
}
func (g *Generator) SetTagsSync(b bool) {
	g.renderTagWithSyncIndex = b
}
func (g *Generator) getSameLabelTags(tag *TagExecNode) []*TagExecNode {
	if g.renderTagWithSyncIndex {
		return g.methodCtx.globalTagNode
	}
	label := tag.data.GetLabels()
	result := []*TagExecNode{}
	for _, l := range label {
		if ms, ok := g.methodCtx.labelTable[l]; ok {
			for m := range ms {
				if m != tag {
					result = append(result, m)
				}
			}
		}
	}
	return result
}
func (g *Generator) generate() (bool, error) {
	if g.first {
		for _, d := range g.data {
			switch ret := d.(type) {
			case *TagExecNode:
				err := ret.FirstExec(false, true, false)
				if err != nil {
					return false, err
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
	failedTag := map[*TagExecNode]struct{}{}
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
		if v, ok := g.data[i].(*TagExecNode); ok {
			for _, m := range g.getSameLabelTags(v) {
				uid1 := reflect.ValueOf(m).UnsafePointer()
				if uid1 == uid { // not allow sync self
					continue
				}
				if !allowSyncTag(v, m) { // check if allow sync by defined rules
					continue
				}
				renderedNode[uid1] = struct{}{}
				ok1, err := m.Exec()
				if err != nil {
					return ok1, err
				}
				genOneOk = genOneOk || ok1 // all label sync render fail then genOneOk false
			}
		}
		if !genOneOk {
			if v, ok := g.data[i].(*TagExecNode); ok {
				failedTag[v] = struct{}{}
			}
		} else { // all render fail try backpropagation
			for _, back := range successCallBacks {
				if err := back(); err != nil {
					return true, err
				}
			}
			var execAllFirst func(t ExecNode)
			execAllFirst = func(t ExecNode) {
				if v1, ok := t.(*TagExecNode); ok && (v1.isDyn || v1.finished) {
					for _, param := range v1.params {
						execAllFirst(param)
					}
					v1.FirstExec(false, true, true) //在这个节点第一次执行时已经判断了err，这里不用判断了
				}
			}
			for tag, _ := range failedTag {
				if _, ok := g.methodCtx.dynTag[tag]; ok {
					continue
				} else {
					tag.FirstExecWithBackpropagation(true, false, true)
					for _, m := range g.getSameLabelTags(tag) {
						uid1 := reflect.ValueOf(m).UnsafePointer()
						if uid1 == uid { // not allow sync self
							continue
						}
						if !allowSyncTag(tag, m) { // check if allow sync by defined rules
							continue
						}
						m.FirstExecWithBackpropagation(true, false, true)
					}
				}
			}
			for tag, _ := range g.methodCtx.dynTag {
				execAllFirst(tag)
				tag.backpropagation()
			}
			isOk = true
			break
		}
		i++
	}
	return isOk, nil
}

func allowSyncTag(srcTag, syncTag *TagExecNode) bool {
	// flowcontrol tag not allow sync
	if syncTag.isFlowControl || srcTag.isFlowControl {
		return false
	}

	// check if sync tag is parent of src tag
	tag1, tag2 := srcTag, syncTag
	for {
		if tag1.parentNode == nil {
			break
		}
		tag1 = tag1.parentNode.(*TagExecNode)
		if tag1 == tag2 {
			return false
		}
	}

	// check if src tag is parent of sync tag
	tag1, tag2 = srcTag, syncTag
	for {
		if tag2.parentNode == nil {
			break
		}
		tag2 = tag2.parentNode.(*TagExecNode)
		if tag1 == tag2 {
			return false
		}
	}
	return true
}

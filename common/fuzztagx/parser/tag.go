package parser

import (
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
	"sync"
)

type FuzzResult struct {
	source  []any // []*FuzzResult | [][]byte
	data    any   // []byte | string
	byTag   bool
	verbose string
	contact bool
}

func NewFuzzResultWithData(d any) *FuzzResult {
	return &FuzzResult{
		data: d,
	}
}
func NewFuzzResultWithDataVerbose(d any, v string) *FuzzResult {
	return &FuzzResult{
		data:    d,
		verbose: v,
	}
}
func NewFuzzResult() *FuzzResult {
	return &FuzzResult{}
}

func (f *FuzzResult) GetData() []byte {
	switch ret := f.data.(type) {
	case []byte:
		return ret
	case string:
		return []byte(ret)
	default:
		return utils.InterfaceToBytes(ret)
	}
}
func (f *FuzzResult) GetVerbose() []string {
	var verboses []string
	for _, datum := range f.source {
		switch ret := datum.(type) {
		case *FuzzResult:
			verboses = append(verboses, ret.GetVerbose()...)
		}
	}
	if !f.byTag {
		return verboses
	}
	if f.verbose == "" {
		f.verbose = utils.InterfaceToString(f.data)
	}
	return append([]string{f.verbose}, verboses...)
}

type TagMethod struct {
	Name  string
	IsDyn bool
	Fun   func(string) ([]*FuzzResult, error)
}
type Node interface {
	IsNode()
}

type StringNode string

func (s StringNode) IsNode() {
}

type TagNode interface {
	IsNode()
	Exec(*FuzzResult, ...map[string]*TagMethod) ([]*FuzzResult, error)
	AddData(node ...Node)
	AddLabel(label string)

	GetData() []Node
	GetLabels() []string
}

type BaseTag struct {
	Data    []Node
	Labels  []string
	Methods *map[string]*TagMethod
	// initOnce 用来对子结构体的初始化
	initOnce sync.Once
}

func (*BaseTag) IsNode() {

}

// DoOnce 用来对子结构体的初始化，可以在Exec函数中调用
func (b *BaseTag) DoOnce(f func()) {
	b.initOnce.Do(f)
}
func (b *BaseTag) AddLabel(label string) {
	b.Labels = append(b.Labels, label)
}
func (b *BaseTag) AddData(node ...Node) {
	b.Data = append(b.Data, node...)
}

func (b *BaseTag) GetData() []Node {
	return b.Data
}

func (b *BaseTag) GetLabels() []string {
	return b.Labels
}

// TagDefine 自定义tag类型
type TagDefine struct {
	name      string
	start     string
	end       string
	tagStruct TagNode
	raw       bool
}

func (t *TagDefine) NewTag() TagNode {
	return reflect.New(reflect.ValueOf(t.tagStruct).Type().Elem()).Interface().(TagNode)
}

// NewTagDefine hooks参数用于判断数据是否需要解析tag
func NewTagDefine(name, start, end string, tagStruct TagNode, raw ...bool) *TagDefine {
	var r bool
	if len(raw) > 0 {
		r = raw[0]
	}
	return &TagDefine{
		name:      name,
		start:     start,
		end:       end,
		tagStruct: tagStruct,
		raw:       r,
	}
}

package parser

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

type FuzzResult struct {
	Source  []any // []*FuzzResult | [][]byte
	Data    any   // []byte | string
	ByTag   bool
	Verbose string
	Contact bool
	Error   error
}

func NewFuzzResultWithData(d any) *FuzzResult {
	return &FuzzResult{
		Data: d,
	}
}
func NewFuzzResultWithDataVerbose(d any, v string) *FuzzResult {
	return &FuzzResult{
		Data:    d,
		Verbose: v,
	}
}
func NewFuzzResult() *FuzzResult {
	return &FuzzResult{}
}

func (f *FuzzResult) GetData() []byte {
	switch ret := f.Data.(type) {
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
	for _, datum := range f.Source {
		switch ret := datum.(type) {
		case *FuzzResult:
			verboses = append(verboses, ret.GetVerbose()...)
		}
	}
	if !f.ByTag {
		return verboses
	}
	if f.Verbose == "" {
		f.Verbose = utils.InterfaceToString(f.Data)
	}
	return append([]string{f.Verbose}, verboses...)
}

type TagMethod struct {
	Name          string
	IsDyn         bool
	IsFlowControl bool
	Fun           func(string) ([]*FuzzResult, error)
	YieldFun      func(ctx context.Context, params string, yield func(*FuzzResult)) error
	Expand        map[string]any
	Alias         []string
	Description   string
}
type Node interface {
	IsNode()
	String() string
}

type StringNode string

func (s StringNode) IsNode() {
}
func (s StringNode) String() string {
	return string(s)
}
func (s StringNode) IsFlowControl() bool {
	return false
}

type TagNode interface {
	IsNode()
	Exec(ctx context.Context, raw *FuzzResult, yield func(*FuzzResult), methodTable map[string]*TagMethod) error
	//Exec(*FuzzResult, func(*FuzzResult), ...map[string]*TagMethod) error
	AddData(node ...Node)
	AddLabel(label string)
	String() string
	GetData() []Node
	GetLabels() []string
	IsFlowControl() bool
}

type BaseTag struct {
	Data   []Node
	Labels []string
	// initOnce 用来对子结构体的初始化
	isFlowControl bool
	initOnce      sync.Once
}

func (b *BaseTag) IsFlowControl() bool {
	return b.isFlowControl
}

func (*BaseTag) IsNode() {

}
func (b *BaseTag) String() string {
	s := ""
	escaper := NewDefaultEscaper(`\`, "{{", "}}")
	for _, data := range b.Data {
		switch data.(type) {
		case StringNode:
			s += escaper.Escape(data.String())
		default:
			s += data.String()
		}
	}
	return fmt.Sprintf("{{%s}}", s)
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
	dest := reflect.New(reflect.ValueOf(t.tagStruct).Type().Elem()).Interface()

	srcValue := reflect.ValueOf(t.tagStruct).Elem()
	destValue := reflect.ValueOf(dest).Elem()

	for i := 0; i < srcValue.NumField(); i++ {
		destField := destValue.Field(i)
		srcField := srcValue.Field(i)
		destField.Set(srcField)
	}
	return dest.(TagNode)
	//return reflect.New(reflect.ValueOf(t.tagStruct).Type().Elem()).Interface().(TagNode)
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

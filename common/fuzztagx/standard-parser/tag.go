package standard_parser

import (
	"reflect"
	"sync"
)

type FuzzResult []byte

type TagMethod func(string) ([]FuzzResult, error)
type Node interface {
	IsNode()
}

type StringNode string

func (s StringNode) IsNode() {
}

type TagNode interface {
	IsNode()
	Exec(FuzzResult, ...map[string]TagMethod) ([]FuzzResult, error)
	AddData(node ...Node)
	AddLabel(label string)

	GetData() []Node
	GetLabels() []string
}

type BaseTag struct {
	Data    []Node
	Labels  []string
	Methods *map[string]TagMethod
	// initOnce 用来对子结构体的初始化
	initOnce sync.Once
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
}

func (t *TagDefine) NewTag() TagNode {
	return reflect.New(reflect.ValueOf(t.tagStruct).Type().Elem()).Interface().(TagNode)
}

// NewTagDefine hooks参数用于判断数据是否需要解析tag
func NewTagDefine(name, start, end string, tagStruct TagNode) *TagDefine {
	return &TagDefine{
		name:      name,
		start:     start,
		end:       end,
		tagStruct: tagStruct,
	}
}

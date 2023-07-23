package fuzztagx

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"strings"
)

type Node interface {
	Strings() []string
	GenerateOne(n int) (string, error)
}

// String
type StringNode string

func NewStringNode(s string) *StringNode {
	n := StringNode(s)
	return &n
}
func (s *StringNode) Strings() []string {
	return []string{string(*s)}
}
func (s *StringNode) GenerateOne(n int) (string, error) {
	if n > 0 {
		return "", nil
	}
	return string(*s), nil
}

// String
type ExpressionNode string

func NewExpressionNode(s string) *ExpressionNode {
	n := ExpressionNode(s)
	return &n
}
func (s *ExpressionNode) Strings() []string {
	return []string{string(*s)}
}
func (f *ExpressionNode) GenerateOne(n int) (string, error) {
	box := httptpl.NewNucleiDSLYakSandbox()
	res, err := box.Execute(string(*f))
	if err != nil {
		return "", nil
	} else {
		return utils.InterfaceToString(res), nil
	}
}

// FuzzTag/ExpressionTag
type Tag struct {
	IsExpTag bool
	Nodes    []Node
}

func (f *Tag) Strings() []string {
	return nil
}
func (f *Tag) GenerateOne(n int) (string, error) {
	res := ""
	for _, node := range f.Nodes {
		d, err := node.GenerateOne(n)
		if err == nil {
			res += d
		}
	}
	return res, nil
}

// FuzzTagMethod
type FuzzTagMethod struct {
	catch  *[]string
	name   string
	labels map[string]struct{}
	params []Node
}

func (f *FuzzTagMethod) Strings() []string {
	return nil
}
func (f *FuzzTagMethod) ParseLabel() {
	labels := strings.Split(f.name, "::")
	f.name = labels[0]
	for _, label := range labels[1:] {
		f.labels[label] = struct{}{}
	}
}

func (f *FuzzTagMethod) GenerateOne(n int) (string, error) {
	res := ""
	for _, param := range f.params {
		if d, err := param.GenerateOne(n); err == nil {
			res += d
		}
	}

	_, isDyn := f.labels["dynamic"]
	_, isRepeat := f.labels["repeat"]
	if f.catch == nil || isDyn {
		if v, ok := BuildInTag[f.name]; ok {
			res := v(res)
			f.catch = &res
		} else {
			return "", nil
		}
	}
	if n < len(*f.catch) {
		return (*f.catch)[n], nil
	} else {
		if isRepeat {
			return utils.GetLastElement(*f.catch), nil
		}
		return "", utils.Error("index out bound")
	}
}

package fuzztagx

import "github.com/yaklang/yaklang/common/utils"

type DataContext struct {
	data         []Node
	deep         int
	unscanstr    string
	stack        *utils.Stack[any]
	currentIndex int
	preIndex     int
	currentByte  byte
	source       string
	toState      state
	currentState state
	transOk      bool
	token        string
	funTable     *map[string]BuildInTagFun
}

func (d *DataContext) SetIndex(i int) {
	d.currentIndex = i
}

//	func (d *DataContext) GetString() string {
//		s := d.source[d.preIndex:d.currentIndex]
//		d.preIndex = d.currentIndex
//		return s
//	}
func NewDataContext(source string) *DataContext {
	return &DataContext{source: source, stack: utils.NewStack[any]()}
}
func (d *DataContext) Generate() ([]string, error) {
	return nil, nil
}
func (d *DataContext) PushData(data Node) {
	switch ret := data.(type) {
	case *StringNode:
		if ret.data == "" {
			return
		}
		last := utils.GetLastElement(d.data)
		if v2, ok := last.(*StringNode); ok {
			v2.data += ret.data
			return
		}
	}
	d.data = append(d.data, data)
}

func (d *DataContext) PushToStack(data any) {
	d.deep++
	i := d.preIndex
	switch data.(type) {
	case *Tag:
		i -= 2
	default:
		i -= 1
	}
	d.stack.Push(i)
	d.stack.Push(data)
}
func (d *DataContext) Pop() (any, int) {
	d.deep--
	return d.stack.Pop(), d.stack.Pop().(int)
}

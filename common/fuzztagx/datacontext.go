package fuzztagx

import "github.com/yaklang/yaklang/common/utils"

type DataContext struct {
	data         []any
	deep         int
	unscanstr    string
	stack        *utils.Stack
	currentIndex int
	preIndex     int
	currentByte  byte
	source       string
	toState      state
	currentState state
	transOk      bool
	token        string
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
	return &DataContext{source: source, stack: utils.NewStack()}
}
func (d *DataContext) Generate() ([]string, error) {
	return nil, nil
}
func (d *DataContext) PushData(data any) {
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

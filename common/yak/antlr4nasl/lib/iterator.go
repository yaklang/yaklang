package lib

import (
	"reflect"
	"yaklang.io/yaklang/common/utils"
)

type Iterator struct {
	data  *reflect.Value
	index int
}

func NewIterator(v interface{}) (*Iterator, error) {
	refV := reflect.ValueOf(v)
	if !(refV.Type().Kind() == reflect.Array || refV.Type().Kind() == reflect.Slice) {
		return nil, utils.Error("not support")
	}
	return &Iterator{
		data:  &refV,
		index: 0,
	}, nil
}
func (i *Iterator) Next() (interface{}, bool) {
	if i.index >= i.data.Len() {
		return nil, false
	}
	v := i.data.Index(i.index)
	i.index++
	return v.Interface(), true
}

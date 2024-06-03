package lib

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/executor/nasl_type"
	"reflect"
)

type IteratorInterface interface {
	Next() (interface{}, bool)
}
type Iterator struct {
	data  *reflect.Value
	index int
}
type NaslIterator struct {
	data  *nasl_type.NaslArray
	index int
}

func NewIterator(v interface{}) (IteratorInterface, error) {
	if v == nil {
		return nil, utils.Error("not support iterator nil")
	}
	if val, ok := v.(*nasl_type.NaslArray); ok {
		return &NaslIterator{
			data:  val,
			index: 0,
		}, nil
	}
	refV := reflect.ValueOf(v)
	if !(refV.Type().Kind() == reflect.Array || refV.Type().Kind() == reflect.Slice) {
		return nil, utils.Error("not support")
	}
	return &Iterator{
		data:  &refV,
		index: 0,
	}, nil
}
func (i *NaslIterator) Next() (interface{}, bool) {
	if i == nil {
		return nil, false
	}
	if i.index >= len(i.data.Num_elt) {
		return nil, false
	}
	v := i.data.Num_elt[i.index]
	i.index++
	return v, true
}
func (i *Iterator) Next() (interface{}, bool) {
	if i == nil {
		return nil, false
	}
	if i.index >= i.data.Len() {
		return nil, false
	}
	v := i.data.Index(i.index)
	i.index++
	return v.Interface(), true
}

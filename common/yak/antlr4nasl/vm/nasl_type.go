package vm

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
)

type NaslType interface {
	Interface() interface{}
}
type NaslData []byte
type NaslInt int64
type NaslString string
type NaslArray struct {
	Max_idx  int
	Hash_elt map[string]interface{}
	Num_elt  map[int]interface{}
}

func (n *NaslData) Interface() interface{} {
	return *n
}
func (n *NaslArray) Interface() interface{} {
	return n
}
func (n *NaslInt) Interface() interface{} {
	return *n
}
func (n *NaslString) Interface() interface{} {
	return *n
}
func NewNaslData(d []byte) *NaslData {
	return (*NaslData)(&d)
}
func NewNaslInt(i int64) *NaslInt {
	return (*NaslInt)(&i)
}
func NewNaslString(s string) *NaslString {
	return (*NaslString)(&s)
}
func NewEmptyNaslArray() *NaslArray {
	return &NaslArray{
		Max_idx:  0,
		Hash_elt: make(map[string]interface{}),
		Num_elt:  make(map[int]interface{}),
	}
}
func (n *NaslArray) AddEleToList(index int, ele interface{}) error {
	if index < 0 {
		err := utils.Error("add_var_to_list: negative index are not (yet) supported\n")
		log.Error(err)
		return err
	}
	if index > n.Max_idx {
		n.Max_idx = index + 1
	} else {
		n.Max_idx += 1
	}
	n.Num_elt[index] = ele
	return nil
}
func (n *NaslArray) AddEleToArray(index string, ele interface{}) error {
	n.Hash_elt[index] = ele
	return nil
}

func NewNaslArray(data interface{}) (*NaslArray, error) {
	res := NewEmptyNaslArray()
	if data == nil {
		return res, nil
	}
	refV := reflect.ValueOf(data)
	if refV.Type().Kind() == reflect.Array || refV.Type().Kind() == reflect.Slice {
		for i := 0; i < refV.Len(); i++ {
			if err := res.AddEleToList(i, refV.Index(i).Interface()); err != nil {
				return nil, err
			}
		}
	} else if refV.Type().Kind() == reflect.Map {
		switch refV.Type().Key().Kind() {
		case reflect.String:
			for _, k := range refV.MapKeys() {
				if err := res.AddEleToArray(k.String(), refV.MapIndex(k).Interface()); err != nil {
					return nil, err
				}
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			for _, k := range refV.MapKeys() {
				if err := res.AddEleToList(int(k.Int()), refV.MapIndex(k).Interface()); err != nil {
					return nil, err
				}
			}
		default:
			return nil, utils.Error("not support")
		}
	}
	return res, nil
}

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
	max_idx  int
	Hash_elt map[string]interface{}
	Num_elt  []interface{}
}
type SortableArrayByString []interface{}

func (a SortableArrayByString) Len() int { return len(a) }
func (a SortableArrayByString) Less(i, j int) bool {
	return utils.InterfaceToString(a[i]) < utils.InterfaceToString(a[j])
}
func (a SortableArrayByString) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

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
		max_idx:  0,
		Hash_elt: make(map[string]interface{}),
	}
}
func (n *NaslArray) Copy() *NaslArray {
	res := NewEmptyNaslArray()
	res.max_idx = n.max_idx
	for k, v := range n.Hash_elt {
		res.Hash_elt[k] = v
	}
	for _, v := range n.Num_elt {
		res.Num_elt = append(res.Num_elt, v)
	}
	return res
}
func (n *NaslArray) AddEleToList(index int, ele interface{}) error {
	//if ele == nil {
	//	return nil
	//}
	if index < 0 {
		err := utils.Error("add_var_to_list: negative index are not (yet) supported\n")
		log.Error(err)
		return err
	}
	if index > n.max_idx {
		n.max_idx = index + 1
	} else {
		n.max_idx += 1
	}
	for len(n.Num_elt) < n.max_idx {
		n.Num_elt = append(n.Num_elt, nil)
	}
	n.Num_elt[index] = ele
	return nil
}

func (n *NaslArray) GetMaxIdx() int {
	return n.max_idx
}
func (n *NaslArray) AddEleToArray(index string, ele interface{}) error {
	n.Hash_elt[index] = ele
	return nil
}

func (n *NaslArray) GetElementByNum(i int) interface{} {
	if i < 0 || i >= len(n.Num_elt) {
		return nil
	}
	return n.Num_elt[i]
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
	} else {
		return nil, utils.Error("not support")
	}
	return res, nil
}

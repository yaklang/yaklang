package sfvm

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strconv"
)

type Value[V any] struct {
	v    any
	data *omap.OrderedMap[string, V]
}

func NewValue[V any](v any) *Value[V] {
	switch v.(type) {
	case *omap.OrderedMap[string, V]:
		return &Value[V]{v: "", data: v.(*omap.OrderedMap[string, V])}
	}
	return &Value[V]{v: v, data: nil}
}

func (v *Value[V]) AsInt() int {
	return utils.InterfaceToInt(v.v)
}

func (v *Value[V]) AsMap() *omap.OrderedMap[string, V] {
	return v.data
}

func (v *Value[V]) AsString() string {
	return utils.InterfaceToString(v.v)
}

func (v *Value[V]) AsBool() bool {
	return utils.InterfaceToBoolean(v.v)
}

func (v *Value[V]) IsMap() bool {
	return v.data != nil
}

func (v *Value[V]) Value() any {
	if v.IsMap() {
		return v.data
	}
	return v.v
}

func (v *Value[V]) VerboseString() string {
	if v.IsMap() {
		return fmt.Sprintf("(len: %v) omap: {...}", v.data.Len())
	}

	switch ret := v.v.(type) {
	case string:
		return strconv.Quote(ret)
	case int:
		return strconv.Itoa(ret)
	case bool:
		return strconv.FormatBool(ret)
	}

	//fallback
	return fmt.Sprintf("verbose: %v", spew.Sdump(v.v))
}

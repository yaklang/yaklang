package sfvm

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type Value[T comparable, V any] struct {
	v    any
	data *omap.OrderedMap[T, V]
}

func NewValue[T comparable, V any](v any) *Value[T, V] {
	switch v.(type) {
	case *omap.OrderedMap[T, V]:
		return &Value[T, V]{v: "", data: v.(*omap.OrderedMap[T, V])}
	}
	return &Value[T, V]{v: v, data: omap.NewEmptyOrderedMap[T, V]()}
}

func (v *Value[T, V]) AsInt() int {
	return utils.InterfaceToInt(v.v)
}

func (v *Value[T, V]) AsMap() *omap.OrderedMap[T, V] {
	return v.data
}

func (v *Value[T, V]) AsString() string {
	return utils.InterfaceToString(v.v)
}

func (v *Value[T, V]) AsBool() bool {
	return utils.InterfaceToBoolean(v.v)
}

func (v *Value[T, V]) Filter() *omap.OrderedMap[T, V] {
	return v.data
}

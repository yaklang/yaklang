package yak

import (
	"reflect"

	"github.com/yaklang/yaklang/common/synscanx"
)

// appendSynScanOptions keeps every variadic synscan option. The Yak VM passes
// them either one argument at a time or as a single slice. Treating only the
// last argument as a slice drops that option.
func appendSynScanOptions(funcType reflect.Type, args []reflect.Value, prepend []synscanx.SynxConfigOption) []reflect.Value {
	opts := append([]synscanx.SynxConfigOption{}, prepend...)
	nFixed := funcType.NumIn()
	if funcType.IsVariadic() {
		nFixed--
	}
	fixed := make([]reflect.Value, 0, len(args)+len(opts))
	for i, arg := range args {
		if i < nFixed {
			fixed = append(fixed, arg)
			continue
		}
		if !arg.IsValid() || !arg.CanInterface() {
			continue
		}
		switch v := arg.Interface().(type) {
		case synscanx.SynxConfigOption:
			opts = append(opts, v)
		case []synscanx.SynxConfigOption:
			opts = append(opts, v...)
		default:
			fixed = append(fixed, arg)
		}
	}
	for _, opt := range opts {
		fixed = append(fixed, reflect.ValueOf(opt))
	}
	return fixed
}

package yaklib

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/reducer"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

func intersect(x interface{}, y interface{}) interface{} {
	if !funk.IsCollection(x) {
		panic("First parameter must be a collection")
	}
	if !funk.IsCollection(y) {
		panic("Second parameter must be a collection")
	}

	hash := map[interface{}]struct{}{}

	xValue := reflect.ValueOf(x)
	xType := xValue.Type()

	yValue := reflect.ValueOf(y)
	yType := yValue.Type()

	if funk.NotEqual(xType, yType) {
		panic("Parameters must have the same type")
	}

	zType := reflect.SliceOf(xType.Elem())
	zSlice := reflect.MakeSlice(zType, 0, 0)

	for i := 0; i < xValue.Len(); i++ {
		v := xValue.Index(i).Interface()
		hash[v] = struct{}{}
	}

	for i := 0; i < yValue.Len(); i++ {
		v := yValue.Index(i).Interface()
		_, ok := hash[v]
		if ok {
			zSlice = reflect.Append(zSlice, yValue.Index(i))
		}
	}

	return zSlice.Interface()
}

type (
	funkGeneralFuncType       func(i interface{}) interface{}
	funkGeneralReduceFuncType func(interface{}, interface{}) interface{}
)

var FunkExports = map[string]interface{}{
	"Retry":       utils.Retry2,
	"WaitConnect": WaitConnect,
	//"WaitConnect": WaitConnect,
	"Map": func(i interface{}, fc funkGeneralFuncType) interface{} {
		return funk.Map(i, func(i interface{}) interface{} {
			return fc(i)
		})
	},
	"ToMap": funk.ToMap,
	"Reduce": func(i interface{}, fc funkGeneralReduceFuncType, acc interface{}) interface{} {
		return funk.Reduce(i, func(pre interface{}, after interface{}) interface{} {
			return fc(pre, after)
		}, acc)
	},
	"Filter": func(i interface{}, fc func(interface{}) bool) interface{} {
		return funk.Filter(i, func(pre interface{}) bool {
			return fc(pre)
		})
	},
	"Find": func(i interface{}, fc func(interface{}) bool) interface{} {
		return funk.Find(i, func(pre interface{}) bool {
			return fc(pre)
		})
	},
	"Foreach": func(i interface{}, fc func(interface{})) {
		funk.ForEach(i, func(pre interface{}) {
			fc(pre)
		})
	},
	"ForeachRight": func(i interface{}, fc func(interface{})) {
		funk.ForEachRight(i, func(pre interface{}) {
			fc(pre)
		})
	},
	"Contains":     funk.Contains,
	"IndexOf":      funk.IndexOf,
	"Difference":   funk.Difference,
	"Subtract":     funk.Subtract,
	"Intersect":    intersect,
	"IsSubset":     funk.Subset,
	"Equal":        funk.IsEqual,
	"Chunk":        funk.Chunk,
	"RemoveRepeat": funk.Uniq,
	"Tail":         funk.Tail,
	"Head":         funk.Head,
	"Drop":         funk.Drop,
	"Shift": func(i interface{}) interface{} {
		return funk.Drop(i, 1)
	},
	"Values":    funk.Values,
	"Keys":      funk.Keys,
	"Zip":       funk.Zip,
	"ToFloat64": funk.ToFloat64,
	"Shuffle":   funk.Shuffle,
	"Reverse":   funk.Reverse,
	"Sum":       funk.Sum,
	"All":       funk.All,
	"Max":       max,
	"Min":       min,
	"Some":      funk.Some,
	"Every":     funk.Every,
	"Any":       funk.Any,
	"Sort":      sort.SliceStable,
	"Range": func(i int) []interface{} {
		return make([]interface{}, i)
	},
	"If": func(i bool, a, b interface{}) interface{} {
		if i {
			return a
		} else {
			return b
		}
	},
	"ConvertToMap": func(i interface{}) map[string][]string {
		return utils.InterfaceToMap(i)
	},
	"GC": func() {
		debug.SetGCPercent(8)
		runtime.GC()
		debug.FreeOSMemory()
		debug.SetGCPercent(8)
	},
	"GCPercent":       debug.SetGCPercent,
	"NewReducer":      reducer.NewReducer,
	"NewEventWatcher": utils.NewEntityWatcher,
}

func min(i interface{}) interface{} {
	if !funk.IsCollection(i) {
		panic("not a valid collection")
	}

	switch ret := i.(type) {
	case []string:
		return funk.MinString(ret)
	case []int:
		return funk.MinInt(ret)
	case []float64:
		return funk.MinFloat64(ret)
	case []int8:
		return funk.MinInt8(ret)
	case []int16:
		return funk.MinInt16(ret)
	case []int32:
		return funk.MinInt32(ret)
	case []int64:
		return funk.MinInt64(ret)
	default:
		panic(fmt.Sprintf("cannot support type: %v", reflect.TypeOf(ret)))
		return nil
	}
}

func max(i interface{}) interface{} {
	if !funk.IsCollection(i) {
		panic("not a valid collection")
	}

	switch ret := i.(type) {
	case []string:
		return funk.MaxString(ret)
	case []int:
		return funk.MaxInt(ret)
	case []float64:
		return funk.MaxFloat64(ret)
	case []int8:
		return funk.MaxInt8(ret)
	case []int16:
		return funk.MaxInt16(ret)
	case []int32:
		return funk.MaxInt32(ret)
	case []int64:
		return funk.MaxInt64(ret)
	default:
		panic(fmt.Sprintf("cannot support type: %v", reflect.TypeOf(ret)))
		return nil
	}
}

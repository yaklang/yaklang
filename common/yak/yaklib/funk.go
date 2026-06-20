package yaklib

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils/reducer"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"time"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

// Intersect 返回两个集合的交集(同时存在于两个集合中的元素)
// 参数:
//   - x: 第一个集合
//   - y: 第二个集合
//
// 返回值:
//   - 两个集合的交集
//
// Example:
// ```
// // VARS: 求两个切片的交集
// result = x.Intersect([1, 2, 3], [2, 3, 4])
// // STDOUT: 打印交集
// println(result)   // OUT: [2 3]
// // assert: 锁定结论
// assert len(result) == 2, "intersection of the two slices has 2 elements"
// ```
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

// funkRetry 反复调用 handler，直到 handler 返回 false 或达到最大次数（导出名为 x.Retry）
// handler 返回 true 表示"继续重试"，返回 false 表示"停止"
//
// 参数:
//   - i: 最大重试次数
//   - handler: 每次重试调用的函数，返回 true 继续、false 停止
//
// Example:
// ```
// count = 0
// x.Retry(10, () => { count++; return count < 3 })
// println(count)   // OUT: 3
// assert count == 3, "Retry keeps calling while handler returns true"
// ```
func funkRetry(i int, handler func() bool) {
	utils.Retry2(i, handler)
}

// funkSort 使用自定义的 less 比较函数对切片做稳定原地排序（导出名为 x.Sort）
// less(i, j) 返回 true 表示下标 i 的元素应排在下标 j 之前
//
// 参数:
//   - x: 待排序的切片(原地修改)
//   - less: 比较函数，接收两个下标，返回是否 i 应排在 j 前
//
// Example:
// ```
// arr = [3, 1, 2]
// x.Sort(arr, (i, j) => arr[i] < arr[j])
// println(arr)   // OUT: [1 2 3]
// assert arr[0] == 1 && arr[2] == 3, "Sort should sort the slice ascending in place"
// ```
func funkSort(x any, less func(i, j int) bool) {
	sort.SliceStable(x, less)
}

// funkGCPercent 设置 GC 触发阈值百分比并返回旧值（导出名为 x.GCPercent）
// percent 表示相对上次 GC 后存活堆的增长百分比，越小 GC 越频繁；负值可关闭 GC
//
// 参数:
//   - percent: 新的 GC 阈值百分比
//
// 返回值:
//   - 设置前的旧阈值百分比
//
// Example:
// ```
// old = x.GCPercent(150)
// println(typeof(old).String())   // OUT: int
// assert typeof(old).String() == "int", "GCPercent should return the previous percent as int"
// x.GCPercent(old)
// ```
func funkGCPercent(percent int) int {
	return debug.SetGCPercent(percent)
}

// funkNewReducer 创建一个归并器，超过 reduceLimit 条数据时用 handle 把较旧的数据合并（导出名为 x.NewReducer）
// 常用于把无限增长的历史数据压缩到有限规模
//
// 参数:
//   - reduceLimit: 触发归并的数据条数阈值
//   - handle: 归并函数，接收一组字符串并返回合并后的单条字符串
//
// 返回值:
//   - 归并器对象，可调用 Push 推入数据、GetData 获取当前数据
//
// Example:
// ```
// r = x.NewReducer(2, items => str.Join(items, ","))
// r.Push("a"); r.Push("b"); r.Push("c")
// data = r.GetData()
// println(data)
// assert len(data) >= 1, "reducer should keep reduced data"
// ```
func funkNewReducer(reduceLimit int, handle reducer.ReduceFunction) *reducer.Reducer {
	return reducer.NewReducer(reduceLimit, handle)
}

// funkNewEventWatcher 创建一个事件观察器，按时间间隔或累计事件数触发回调（导出名为 x.NewEventWatcher）
//
// 参数:
//   - ctx: 上下文，用于控制观察器生命周期
//   - triggerTime: 触发的时间间隔
//   - triggerCount: 触发的累计事件数阈值
//
// 返回值:
//   - 事件观察器管理对象
//
// Example:
// ```
// d = time.ParseDuration("1s")~
// w = x.NewEventWatcher(context.Background(), d, 10)
// assert w != nil, "event watcher should be created"
// ```
func funkNewEventWatcher(ctx context.Context, triggerTime time.Duration, triggerCount int) *utils.EventWatcherManager {
	return utils.NewEntityWatcher(ctx, triggerTime, triggerCount)
}

var FunkExports = map[string]interface{}{
	"Retry":           funkRetry,
	"WaitConnect":     WaitConnect,
	"Map":             funkMap,
	"ToMap":           funk.ToMap,
	"Reduce":          funkReduce,
	"Filter":          funkFilter,
	"Find":            funkFind,
	"Foreach":         funkForeach,
	"ForeachRight":    funkForeachRight,
	"Contains":        funk.Contains,
	"IndexOf":         funk.IndexOf,
	"Difference":      funk.Difference,
	"Subtract":        funk.Subtract,
	"Intersect":       intersect,
	"IsSubset":        funk.Subset,
	"Equal":           funk.IsEqual,
	"Chunk":           funk.Chunk,
	"RemoveRepeat":    funk.Uniq,
	"Tail":            funk.Tail,
	"Head":            funk.Head,
	"Drop":            funk.Drop,
	"Shift":           funkShift,
	"Values":          funk.Values,
	"Keys":            funk.Keys,
	"Zip":             funk.Zip,
	"ToFloat64":       funk.ToFloat64,
	"Shuffle":         funk.Shuffle,
	"Reverse":         funk.Reverse,
	"Sum":             funk.Sum,
	"All":             funk.All,
	"Max":             max,
	"Min":             min,
	"Some":            funk.Some,
	"Every":           funk.Every,
	"Any":             funk.Any,
	"Sort":            funkSort,
	"Range":           funkRange,
	"If":              funkIf,
	"ConvertToMap":    funkConvertToMap,
	"GC":              funkGC,
	"GCPercent":       funkGCPercent,
	"NewReducer":      funkNewReducer,
	"NewEventWatcher": funkNewEventWatcher,
}

// Map 遍历集合中的每个元素，使用回调函数处理后返回新的切片
// 参数:
//   - i: 待遍历的集合(切片/数组)
//   - fc: 处理每个元素的回调函数，接收元素返回新值
//
// 返回值:
//   - 由回调返回值组成的新切片
//
// Example:
// ```
// // VARS: 把每个元素翻倍
// result = x.Map([1, 2, 3], func(e) { return e * 2 })
// // STDOUT: 打印结果
// println(result)   // OUT: [2 4 6]
// // assert: 元素个数不变
// assert len(result) == 3, "Map should keep element count"
// ```
func funkMap(i interface{}, fc funkGeneralFuncType) interface{} {
	return funk.Map(i, func(i interface{}) interface{} {
		return fc(i)
	})
}

// Reduce 对集合中的元素从初始累加器开始依次归并为单一结果
// 参数:
//   - i: 待归并的集合(切片/数组)
//   - fc: 归并回调，接收累加器与当前元素返回新的累加器
//   - acc: 初始累加器值
//
// 返回值:
//   - 归并后的最终结果
//
// Example:
// ```
// // VARS: 从 0 开始累加求和
// result = x.Reduce([1, 2, 3], func(acc, e) { return acc + e }, 0)
// // STDOUT: 打印结果
// println(result)   // OUT: 6
// // assert: 锁定结论
// assert result == 6, "Reduce should sum the slice to 6"
// ```
func funkReduce(i interface{}, fc funkGeneralReduceFuncType, acc interface{}) interface{} {
	return funk.Reduce(i, func(pre interface{}, after interface{}) interface{} {
		return fc(pre, after)
	}, acc)
}

// Filter 遍历集合，仅保留回调函数返回 true 的元素
// 参数:
//   - i: 待过滤的集合(切片/数组)
//   - fc: 过滤回调，接收元素返回布尔值，true 表示保留
//
// 返回值:
//   - 由保留下来的元素组成的新切片
//
// Example:
// ```
// // VARS: 仅保留偶数
// result = x.Filter([1, 2, 3, 4], func(e) { return e % 2 == 0 })
// // STDOUT: 打印结果
// println(result)   // OUT: [2 4]
// // assert: 过滤后剩 2 个
// assert len(result) == 2, "Filter should keep the two even numbers"
// ```
func funkFilter(i interface{}, fc func(interface{}) bool) interface{} {
	return funk.Filter(i, func(pre interface{}) bool {
		return fc(pre)
	})
}

// Find 遍历集合，返回第一个使回调函数返回 true 的元素
// 参数:
//   - i: 待查找的集合(切片/数组)
//   - fc: 判定回调，接收元素返回布尔值
//
// 返回值:
//   - 第一个满足条件的元素，未找到返回 nil
//
// Example:
// ```
// // VARS: 查找第一个大于 1 的元素
// result = x.Find([1, 2, 3], func(e) { return e > 1 })
// // STDOUT: 打印结果
// println(result)   // OUT: 2
// // assert: 锁定结论
// assert result == 2, "Find should return the first element greater than 1"
// ```
func funkFind(i interface{}, fc func(interface{}) bool) interface{} {
	return funk.Find(i, func(pre interface{}) bool {
		return fc(pre)
	})
}

// Foreach 从前向后遍历集合，对每个元素执行回调函数(无返回值)
// 参数:
//   - i: 待遍历的集合(切片/数组)
//   - fc: 对每个元素执行的回调函数
//
// 返回值:
//   - 无
//
// Example:
// ```
// // VARS: 遍历累加(用闭包收集结果)
// sum = 0
// x.Foreach([1, 2, 3], func(e) { sum += e })
// // STDOUT: 打印累加结果
// println(sum)   // OUT: 6
// // assert: 锁定结论
// assert sum == 6, "Foreach should visit every element"
// ```
func funkForeach(i interface{}, fc func(interface{})) {
	funk.ForEach(i, func(pre interface{}) {
		fc(pre)
	})
}

// ForeachRight 从后向前遍历集合，对每个元素执行回调函数(无返回值)
// 参数:
//   - i: 待遍历的集合(切片/数组)
//   - fc: 对每个元素执行的回调函数
//
// 返回值:
//   - 无
//
// Example:
// ```
// // VARS: 从右向左拼接元素
// order = []
// x.ForeachRight([1, 2, 3], func(e) { order = append(order, e) })
// // STDOUT: 打印访问顺序
// println(order)   // OUT: [3 2 1]
// // assert: 第一个访问的是最后一个元素
// assert order[0] == 3, "ForeachRight should visit from the tail"
// ```
func funkForeachRight(i interface{}, fc func(interface{})) {
	funk.ForEachRight(i, func(pre interface{}) {
		fc(pre)
	})
}

// Shift 返回去掉切片第一个元素后的新切片
// 参数:
//   - i: 源切片
//
// 返回值:
//   - 去掉首元素后的切片
//
// Example:
// ```
// // VARS: 去掉第一个元素
// result = x.Shift([1, 2, 3])
// // STDOUT: 打印结果
// println(result)   // OUT: [2 3]
// // assert: 锁定结论
// assert len(result) == 2, "Shift should drop the first element"
// ```
func funkShift(i interface{}) interface{} {
	return funk.Drop(i, 1)
}

// Range 创建一个长度为 i 的空接口切片，常用于配合 for-range 生成定长循环
// 参数:
//   - i: 切片长度
//
// 返回值:
//   - 长度为 i 的空接口切片
//
// Example:
// ```
// // VARS: 创建长度为 3 的切片
// result = x.Range(3)
// // STDOUT: 打印长度
// println(len(result))   // OUT: 3
// // assert: 锁定结论
// assert len(result) == 3, "Range should create a slice of the given length"
// ```
func funkRange(i int) []interface{} { return make([]interface{}, i) }

// If 三元条件选择，当条件为真时返回 a，否则返回 b
// 参数:
//   - i: 条件布尔值
//   - a: 条件为真时返回的值
//   - b: 条件为假时返回的值
//
// 返回值:
//   - 根据条件选择的值
//
// Example:
// ```
// // VARS: 条件为真时取第一个值
// result = x.If(true, "a", "b")
// // STDOUT: 打印结果
// println(result)   // OUT: a
// // assert: 条件为假时取第二个值
// assert x.If(false, "a", "b") == "b", "If should pick the second value when false"
// ```
func funkIf(i bool, a, b interface{}) interface{} {
	if i {
		return a
	} else {
		return b
	}
}

// ConvertToMap 将传入的对象转换为 map[string][]string 结构，常用于归一化键值数据
// 参数:
//   - i: 待转换的对象(map 或结构体)
//
// 返回值:
//   - 转换后的 map[string][]string
//
// Example:
// ```
// // VARS: 转换为字符串列表映射
// m = x.ConvertToMap({"k": "v"})
// // STDOUT: 打印键对应的值列表
// println(m["k"])   // OUT: [v]
// // assert: 取出第一个值
// assert m["k"][0] == "v", "ConvertToMap should keep the value under its key"
// ```
func funkConvertToMap(i interface{}) map[string][]string {
	return utils.InterfaceToMap(i)
}

// GC 主动触发一次垃圾回收并尽量把空闲内存归还操作系统
// 返回值:
//   - 无
//
// Example:
// ```
// // 主动触发一次垃圾回收(仅副作用，无返回值)
// x.GC()
// ```
func funkGC() {
	debug.SetGCPercent(8)
	runtime.GC()
	debug.FreeOSMemory()
	debug.SetGCPercent(8)
}

// Min 返回数值或字符串切片中的最小值
// 参数:
//   - i: 数值或字符串切片
//
// 返回值:
//   - 切片中的最小元素
//
// Example:
// ```
// // VARS: 求切片最小值
// result = x.Min([3, 1, 2])
// // STDOUT: 打印最小值
// println(result)   // OUT: 1
// // assert: 锁定结论
// assert result == 1, "min of 3,1,2 should be 1"
// ```
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

// Max 返回数值或字符串切片中的最大值
// 参数:
//   - i: 数值或字符串切片
//
// 返回值:
//   - 切片中的最大元素
//
// Example:
// ```
// // VARS: 求切片最大值
// result = x.Max([3, 1, 2])
// // STDOUT: 打印最大值
// println(result)   // OUT: 3
// // assert: 锁定结论
// assert result == 3, "max of 3,1,2 should be 3"
// ```
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

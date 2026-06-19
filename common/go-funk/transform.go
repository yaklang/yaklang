package funk

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
)

// Chunk 将切片按指定大小分组，最后一组可能不足 size 个元素
// 参数:
//   - arr: 待分组的切片
//   - size: 每组的元素个数
//
// 返回值:
//   - 分组后的二维切片
//
// Example:
// ```
// // VARS: 按每组 2 个分组
// result = x.Chunk([1, 2, 3, 4, 5], 2)
// // assert: 5 个元素分成 3 组
// assert len(result) == 3, "5 elements in groups of 2 yields 3 chunks"
// ```
func Chunk(arr interface{}, size int) interface{} {
	if !IsIteratee(arr) {
		panic("First parameter must be neither array nor slice")
	}

	if size == 0 {
		return arr
	}

	arrValue := reflect.ValueOf(arr)

	arrType := arrValue.Type()

	resultSliceType := reflect.SliceOf(arrType)

	// Initialize final result slice which will contains slice
	resultSlice := reflect.MakeSlice(resultSliceType, 0, 0)

	itemType := arrType.Elem()

	var itemSlice reflect.Value

	itemSliceType := reflect.SliceOf(itemType)

	length := arrValue.Len()

	for i := 0; i < length; i++ {
		if i%size == 0 || i == 0 {
			if itemSlice.Kind() != reflect.Invalid {
				resultSlice = reflect.Append(resultSlice, itemSlice)
			}

			itemSlice = reflect.MakeSlice(itemSliceType, 0, 0)
		}

		itemSlice = reflect.Append(itemSlice, arrValue.Index(i))

		if i == length-1 {
			resultSlice = reflect.Append(resultSlice, itemSlice)
		}
	}

	return resultSlice.Interface()
}

// ToMap 将一个结构体切片转换为以指定字段值为键的 map
// 参数:
//   - in: 结构体切片
//   - pivot: 作为键的结构体字段名
//
// 返回值:
//   - 以 pivot 字段值为键、结构体为值的 map
//
// Example:
// ```
// // 将结构体切片按字段转为 map(需结构体切片，作示意)
// m = x.ToMap(users, "Id")
// ```
func ToMap(in interface{}, pivot string) interface{} {
	value := reflect.ValueOf(in)

	// input value must be a slice
	if value.Kind() != reflect.Slice {
		panic(fmt.Sprintf("%v must be a slice", in))
	}

	inType := value.Type()

	structType := inType.Elem()

	// retrieve the struct in the slice to deduce key type
	if structType.Kind() == reflect.Ptr {
		structType = structType.Elem()
	}

	field, _ := structType.FieldByName(pivot)

	// value of the map will be the input type
	collectionType := reflect.MapOf(field.Type, inType.Elem())

	// create a map from scratch
	collection := reflect.MakeMap(collectionType)

	for i := 0; i < value.Len(); i++ {
		instance := value.Index(i)
		var field reflect.Value

		if instance.Kind() == reflect.Ptr {
			field = instance.Elem().FieldByName(pivot)
		} else {
			field = instance.FieldByName(pivot)
		}

		collection.SetMapIndex(field, instance)
	}

	return collection.Interface()
}

func mapSlice(arrValue reflect.Value, funcValue reflect.Value) reflect.Value {
	funcType := funcValue.Type()

	if funcType.NumIn() != 1 || funcType.NumOut() == 0 || funcType.NumOut() > 2 {
		panic("Map function with an array must have one parameter and must return one or two parameters")
	}

	arrElemType := arrValue.Type().Elem()

	// Checking whether element type is convertible to function's first argument's type.
	if !arrElemType.ConvertibleTo(funcType.In(0)) {
		panic("Map function's argument is not compatible with type of array.")
	}

	if funcType.NumOut() == 1 {
		// Get slice type corresponding to function's return value's type.
		resultSliceType := reflect.SliceOf(funcType.Out(0))

		// MakeSlice takes a slice kind type, and makes a slice.
		resultSlice := reflect.MakeSlice(resultSliceType, 0, 0)

		for i := 0; i < arrValue.Len(); i++ {
			result := funcValue.Call([]reflect.Value{arrValue.Index(i)})[0]

			resultSlice = reflect.Append(resultSlice, result)
		}

		return resultSlice
	}

	if funcType.NumOut() == 2 {
		// value of the map will be the input type
		collectionType := reflect.MapOf(funcType.Out(0), funcType.Out(1))

		// create a map from scratch
		collection := reflect.MakeMap(collectionType)

		for i := 0; i < arrValue.Len(); i++ {
			results := funcValue.Call([]reflect.Value{arrValue.Index(i)})

			collection.SetMapIndex(results[0], results[1])
		}

		return collection
	}

	return reflect.Value{}
}

func mapMap(arrValue reflect.Value, funcValue reflect.Value) reflect.Value {
	funcType := funcValue.Type()

	if funcType.NumIn() != 2 || funcType.NumOut() == 0 || funcType.NumOut() > 2 {
		panic("Map function with a map must have two parameters and must return one or two parameters")
	}

	// Only one returned parameter, should be a slice
	if funcType.NumOut() == 1 {
		// Get slice type corresponding to function's return value's type.
		resultSliceType := reflect.SliceOf(funcType.Out(0))

		// MakeSlice takes a slice kind type, and makes a slice.
		resultSlice := reflect.MakeSlice(resultSliceType, 0, 0)

		for _, key := range arrValue.MapKeys() {
			results := funcValue.Call([]reflect.Value{key, arrValue.MapIndex(key)})

			result := results[0]

			resultSlice = reflect.Append(resultSlice, result)
		}

		return resultSlice
	}

	// two parameters, should be a map
	if funcType.NumOut() == 2 {
		// value of the map will be the input type
		collectionType := reflect.MapOf(funcType.Out(0), funcType.Out(1))

		// create a map from scratch
		collection := reflect.MakeMap(collectionType)

		for _, key := range arrValue.MapKeys() {
			results := funcValue.Call([]reflect.Value{key, arrValue.MapIndex(key)})

			collection.SetMapIndex(results[0], results[1])

		}

		return collection
	}

	return reflect.Value{}
}

// Map manipulates an iteratee and transforms it to another type.
func Map(arr interface{}, mapFunc interface{}) interface{} {
	result := mapFn(arr, mapFunc, "Map")

	if result.IsValid() {
		return result.Interface()
	}

	return nil
}

func mapFn(arr interface{}, mapFunc interface{}, funcName string) reflect.Value {
	if !IsIteratee(arr) {
		panic("First parameter must be an iteratee")
	}

	if !IsFunction(mapFunc) {
		panic("Second argument must be function")
	}

	var (
		funcValue = reflect.ValueOf(mapFunc)
		arrValue  = reflect.ValueOf(arr)
		arrType   = arrValue.Type()
	)

	kind := arrType.Kind()

	if kind == reflect.Slice || kind == reflect.Array {
		return mapSlice(arrValue, funcValue)
	} else if kind == reflect.Map {
		return mapMap(arrValue, funcValue)
	}

	panic(fmt.Sprintf("Type %s is not supported by "+funcName, arrType.String()))
}

// FlatMap manipulates an iteratee and transforms it to a flattened collection of another type.
func FlatMap(arr interface{}, mapFunc interface{}) interface{} {
	result := mapFn(arr, mapFunc, "FlatMap")

	if result.IsValid() {
		return flatten(result).Interface()
	}

	return nil
}

// Flatten flattens a two-dimensional array.
func Flatten(out interface{}) interface{} {
	return flatten(reflect.ValueOf(out)).Interface()
}

func flatten(value reflect.Value) reflect.Value {
	sliceType := value.Type()

	if (value.Kind() != reflect.Slice && value.Kind() != reflect.Array) ||
		(sliceType.Elem().Kind() != reflect.Slice && sliceType.Elem().Kind() != reflect.Array) {
		panic("Argument must be an array or slice of at least two dimensions")
	}

	resultSliceType := sliceType.Elem().Elem()

	resultSlice := reflect.MakeSlice(reflect.SliceOf(resultSliceType), 0, 0)

	length := value.Len()

	for i := 0; i < length; i++ {
		item := value.Index(i)

		resultSlice = reflect.AppendSlice(resultSlice, item)
	}

	return resultSlice
}

// FlattenDeep recursively flattens array.
func FlattenDeep(out interface{}) interface{} {
	return flattenDeep(reflect.ValueOf(out)).Interface()
}

func flattenDeep(value reflect.Value) reflect.Value {
	sliceType := sliceElem(value.Type())

	resultSlice := reflect.MakeSlice(reflect.SliceOf(sliceType), 0, 0)

	return flattenRecursive(value, resultSlice)
}

func flattenRecursive(value reflect.Value, result reflect.Value) reflect.Value {
	length := value.Len()

	for i := 0; i < length; i++ {
		item := value.Index(i)
		kind := item.Kind()

		if kind == reflect.Slice || kind == reflect.Array {
			result = flattenRecursive(item, result)
		} else {
			result = reflect.Append(result, item)
		}
	}

	return result
}

// Shuffle 随机打乱切片中元素的顺序，返回打乱后的新切片
// 参数:
//   - in: 待打乱的切片
//
// 返回值:
//   - 打乱顺序后的切片(元素不变)
//
// Example:
// ```
// // VARS: 随机打乱(每次顺序不同)
// result = x.Shuffle([1, 2, 3, 4, 5])
// // assert: 元素个数不变
// assert len(result) == 5, "shuffle should preserve length"
// ```
func Shuffle(in interface{}) interface{} {
	value := reflect.ValueOf(in)
	valueType := value.Type()

	kind := value.Kind()

	if kind == reflect.Array || kind == reflect.Slice {
		length := value.Len()

		resultSlice := makeSlice(value, length)

		for i, v := range rand.Perm(length) {
			resultSlice.Index(i).Set(value.Index(v))
		}

		return resultSlice.Interface()
	}

	panic(fmt.Sprintf("Type %s is not supported by Shuffle", valueType.String()))
}

// Reverse 反转切片中元素的顺序，返回反转后的新切片
// 参数:
//   - in: 待反转的切片
//
// 返回值:
//   - 反转顺序后的切片
//
// Example:
// ```
// // VARS: 反转切片
// result = x.Reverse([1, 2, 3])
// // STDOUT: 打印结果
// println(result)   // OUT: [3 2 1]
// // assert: 首元素变为原末元素
// assert result[0] == 3, "reverse should put last element first"
// ```
func Reverse(in interface{}) interface{} {
	value := reflect.ValueOf(in)
	valueType := value.Type()

	kind := value.Kind()

	if kind == reflect.String {
		return ReverseString(in.(string))
	}

	if kind == reflect.Array || kind == reflect.Slice {
		length := value.Len()

		resultSlice := makeSlice(value, length)

		j := 0
		for i := length - 1; i >= 0; i-- {
			resultSlice.Index(j).Set(value.Index(i))
			j++
		}

		return resultSlice.Interface()
	}

	panic(fmt.Sprintf("Type %s is not supported by Reverse", valueType.String()))
}

// RemoveRepeat 对切片去重，返回仅保留首次出现元素的新切片
// 参数:
//   - in: 待去重的切片
//
// 返回值:
//   - 去重后的切片
//
// Example:
// ```
// // VARS: 去除重复元素
// result = x.RemoveRepeat([1, 1, 2, 3, 3])
// // STDOUT: 打印结果
// println(result)   // OUT: [1 2 3]
// // assert: 去重后剩 3 个
// assert len(result) == 3, "duplicates should be removed"
// ```
func Uniq(in interface{}) interface{} {
	value := reflect.ValueOf(in)
	valueType := value.Type()

	kind := value.Kind()

	if kind == reflect.Array || kind == reflect.Slice {
		length := value.Len()

		result := makeSlice(value, 0)

		seen := make(map[interface{}]bool, length)
		j := 0

		for i := 0; i < length; i++ {
			val := value.Index(i)
			v := val.Interface()

			if _, ok := seen[v]; ok {
				continue
			}

			seen[v] = true
			result = reflect.Append(result, val)
			j++
		}

		return result.Interface()
	}

	panic(fmt.Sprintf("Type %s is not supported by Uniq", valueType.String()))
}

// ConvertSlice converts a slice type to another,
// a perfect example would be to convert a slice of struct to a slice of interface.
func ConvertSlice(in interface{}, out interface{}) {
	srcValue := reflect.ValueOf(in)

	dstValue := reflect.ValueOf(out)

	if dstValue.Kind() != reflect.Ptr {
		panic("Second argument must be a pointer")
	}

	dstValue = dstValue.Elem()

	if srcValue.Kind() != reflect.Slice && srcValue.Kind() != reflect.Array {
		panic("First argument must be an array or slice")
	}

	if dstValue.Kind() != reflect.Slice && dstValue.Kind() != reflect.Array {
		panic("Second argument must be an array or slice")
	}

	// returns value that points to dstValue
	direct := reflect.Indirect(dstValue)

	length := srcValue.Len()

	for i := 0; i < length; i++ {
		dstValue = reflect.Append(dstValue, srcValue.Index(i))
	}

	direct.Set(dstValue)
}

// Drop 从切片开头丢弃 n 个元素，返回剩余元素组成的新切片
// 参数:
//   - in: 源切片
//   - n: 从开头丢弃的元素个数
//
// 返回值:
//   - 丢弃后剩余的切片
//
// Example:
// ```
// // VARS: 丢弃开头 2 个元素
// result = x.Drop([1, 2, 3, 4], 2)
// // STDOUT: 打印结果
// println(result)   // OUT: [3 4]
// // assert: 剩余 2 个元素
// assert len(result) == 2, "drop should remove the first n elements"
// ```
func Drop(in interface{}, n int) interface{} {
	value := reflect.ValueOf(in)
	valueType := value.Type()

	kind := value.Kind()

	if kind == reflect.Array || kind == reflect.Slice {
		length := value.Len()

		resultSlice := makeSlice(value, length-n)

		j := 0
		for i := n; i < length; i++ {
			resultSlice.Index(j).Set(value.Index(i))
			j++
		}

		return resultSlice.Interface()

	}

	panic(fmt.Sprintf("Type %s is not supported by Drop", valueType.String()))
}

// Prune returns a copy of "in" that only contains fields in "paths"
// which are looked up using struct field name.
// For lookup paths by field tag instead, use funk.PruneByTag()
func Prune(in interface{}, paths []string) (interface{}, error) {
	return pruneByTag(in, paths, nil /*tag*/)
}

// pruneByTag returns a copy of "in" that only contains fields in "paths"
// which are looked up using struct field Tag "tag".
func PruneByTag(in interface{}, paths []string, tag string) (interface{}, error) {
	return pruneByTag(in, paths, &tag)
}

// pruneByTag returns a copy of "in" that only contains fields in "paths"
// which are looked up using struct field Tag "tag". If tag is nil,
// traverse paths using struct field name
func pruneByTag(in interface{}, paths []string, tag *string) (interface{}, error) {

	inValue := reflect.ValueOf(in)

	ret := reflect.New(inValue.Type()).Elem()

	for _, path := range paths {
		parts := strings.Split(path, ".")
		if err := prune(inValue, ret, parts, tag); err != nil {
			return nil, err
		}
	}
	return ret.Interface(), nil
}

func prune(inValue reflect.Value, ret reflect.Value, parts []string, tag *string) error {

	if len(parts) == 0 {
		// we reached the location that ret needs to hold inValue
		// Note: The value at the end of the path is not copied, maybe we need to change.
		// ret and the original data holds the same reference to this value
		ret.Set(inValue)
		return nil
	}

	inKind := inValue.Kind()

	switch inKind {
	case reflect.Ptr:
		if inValue.IsNil() {
			// TODO validate
			return nil
		}
		if ret.IsNil() {
			// init ret and go to next level
			ret.Set(reflect.New(inValue.Type().Elem()))
		}
		return prune(inValue.Elem(), ret.Elem(), parts, tag)
	case reflect.Struct:
		part := parts[0]
		var fValue reflect.Value
		var fRet reflect.Value
		if tag == nil {
			// use field name
			fValue = inValue.FieldByName(part)
			if !fValue.IsValid() {
				return fmt.Errorf("field name %v is not found in struct %v", part, inValue.Type().String())
			}
			fRet = ret.FieldByName(part)
		} else {
			// search tag that has key equal to part
			found := false
			for i := 0; i < inValue.NumField(); i++ {
				f := inValue.Type().Field(i)
				if key, ok := f.Tag.Lookup(*tag); ok {
					if key == part {
						fValue = inValue.Field(i)
						fRet = ret.Field(i)
						found = true
						break
					}
				}
			}
			if !found {
				return fmt.Errorf("Struct tag %v is not found with key %v", *tag, part)
			}
		}
		// init Ret is zero and go down one more level
		if fRet.IsZero() {
			fRet.Set(reflect.New(fValue.Type()).Elem())
		}
		return prune(fValue, fRet, parts[1:], tag)
	case reflect.Array, reflect.Slice:
		// set all its elements
		length := inValue.Len()
		// init ret
		if ret.IsZero() {
			if inKind == reflect.Slice {
				ret.Set(reflect.MakeSlice(inValue.Type(), length /*len*/, length /*cap*/))
			} else { // array
				ret.Set(reflect.New(inValue.Type()).Elem())
			}
		}
		for j := 0; j < length; j++ {
			if err := prune(inValue.Index(j), ret.Index(j), parts, tag); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("path %v cannot be looked up on kind of %v", strings.Join(parts, "."), inValue.Kind())
	}

	return nil
}

package yakvm

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
)

var (
	literalReflectType_Byte              = reflect.TypeOf(byte(0))
	literalReflectType_Bytes             = reflect.TypeOf([]byte{})
	literalReflectType_String            = reflect.TypeOf("")
	literalReflectType_Int               = reflect.TypeOf(0)
	literalReflectType_Int8              = reflect.TypeOf(int8(0))
	literalReflectType_Int16             = reflect.TypeOf(int16(0))
	literalReflectType_Int32             = reflect.TypeOf(int32(0))
	literalReflectType_Int64             = reflect.TypeOf(int64(0))
	literalReflectType_Uint              = reflect.TypeOf(uint(0))
	literalReflectType_Uint8             = reflect.TypeOf(uint8(0))
	literalReflectType_Uint16            = reflect.TypeOf(uint16(0))
	literalReflectType_Uint32            = reflect.TypeOf(uint32(0))
	literalReflectType_Uint64            = reflect.TypeOf(uint64(0))
	literalReflectType_Float32           = reflect.TypeOf(float32(0.1))
	literalReflectType_Float64           = reflect.TypeOf(float64(0.1))
	literalReflectType_Bool              = reflect.TypeOf(false)
	literalReflectType_Interface         = reflect.TypeOf((*interface{})(nil)).Elem()
	literalReflectType_YakFunction       = reflect.TypeOf(&Function{})
	literalReflectType_NativeFunction    = reflect.TypeOf(func() {})
	literalReflectType_NativeWarpFuntion = reflect.FuncOf([]reflect.Type{reflect.SliceOf(literalReflectType_Interface)}, []reflect.Type{literalReflectType_Interface}, true)
	literalReflectType_OrderedMap        = reflect.TypeOf((*orderedmap.OrderedMap)(nil))
)

/*
关于类型的描述，我们通过一些简单的方式可以保证
 1. 在 golang => yak 的时候，清理到不好的类型，比如 uint 与 float32，其他的类型均是可以接受的
    但是因为有限的情况下，int64 和 int 是可以接受的，但是再大就有点小问题了（需要注意的是，一般 x64 系统中，int64 和 int 是一样的）
 2. 所以在计算过程中 yak => golang 的过程中，我们不希望调用到任何 uint 类型的东西
*/
func IsInt(v interface{}) bool {
	switch v.(type) {
	case int, int64, int8, int16, int32,
		uint, uint8, uint16, uint32, uint64:
		return true
	}
	return false
}

func IsFloat(v interface{}) bool {
	switch v.(type) {
	case float64, float32:
		return true
	}
	return false
}

func GuessBasicType(vals ...interface{}) reflect.Type {
	var (
		anyT  = literalReflectType_Interface
		kindI reflect.Kind
	)
	if len(vals) <= 0 {
		return anyT
	}

	last := anyT
	for index, val := range vals {
		kindI = reflect.ValueOf(val).Kind()
		if index == 0 {
			// 识别第一个类型
			if kindI == reflect.String {
				last = literalReflectType_String
			} else if kindI == reflect.Uint8 {
				last = literalReflectType_Byte
			} else if IsInt(val) {
				last = literalReflectType_Int
			} else if kindI == reflect.Bool {
				last = literalReflectType_Bool
			} else if IsFloat(val) {
				last = literalReflectType_Float64
			}
			continue
		}

		if kindI == reflect.String {
			// 这个类型不存在兼容问题
			if last.Kind() != reflect.String {
				return anyT
			}
		} else if IsInt(val) {
			// 一般来说，Int 和 Float 应该是可以互相转换的，使用最兼容类型
			// 兼容性 float64 是兼容性最高的
			if last.Kind() != reflect.Int {
				if last.Kind() > reflect.Int && last.Kind() <= reflect.Float64 {
					continue
				}
				return anyT
			}
		} else if IsFloat(val) {
			if last.Kind() != reflect.Float64 {
				if last.Kind() >= reflect.Int && last.Kind() < reflect.Float64 {
					last = literalReflectType_Float64
					continue
				}
				return anyT
			}
		} else if kindI == reflect.Bool {
			if last.Kind() != reflect.Bool {
				return anyT
			}
		} else {
			return anyT
		}
	}
	return last
}

func GuessValuesTypeToBasicType(vals ...*Value) reflect.Type {
	anyT := literalReflectType_Interface
	if len(vals) <= 0 {
		return anyT
	}

	last := anyT
	for index, i := range vals {
		if i == nil || i.Value == nil || i.IsUndefined() {
			return anyT
		}
		if index == 0 {
			// 识别第一个类型
			if i.IsByte() {
				last = literalReflectType_Byte
			} else if i.IsString() {
				last = literalReflectType_String
			} else if i.IsBytes() {
				last = literalReflectType_Bytes
			} else if i.IsInt() {
				last = literalReflectType_Int
			} else if i.IsBool() {
				last = literalReflectType_Bool
			} else if i.IsFloat() {
				last = literalReflectType_Float64
			} else if i.Callable() {
				last = literalReflectType_Interface
			} else {
				last = reflect.TypeOf(i.Value)
			}
			continue
		}

		if i.IsUndefined() {
			return anyT
		}

		refTyp := reflect.TypeOf(i.Value)
		refKind := refTyp.Kind()
		if last == refTyp {
			continue
		}
		isStructKind := refKind == reflect.Struct || (refKind == reflect.Pointer && refTyp.Elem().Kind() == reflect.Struct)
		if isStructKind && refTyp.ConvertibleTo(last) {
			continue
		}

		if i.IsStringOrBytes() {
			// 这个类型不存在兼容问题
			if last.Kind() != reflect.String && !i.IsBytes() {
				return anyT
			}
		} else if i.IsInt() {
			// 一般来说，Int 和 Float 应该是可以互相转换的，使用最兼容类型
			// 兼容性 float64 是兼容性最高的
			if last.Kind() != reflect.Int {
				if last.Kind() > reflect.Int && last.Kind() <= reflect.Float64 {
					continue
				}
				return anyT
			}
		} else if i.IsFloat() {
			if last.Kind() != reflect.Float64 {
				if last.Kind() >= reflect.Int && last.Kind() < reflect.Float64 {
					last = literalReflectType_Float64
					continue
				}
				return anyT
			}
		} else if i.IsBool() {
			if last.Kind() != reflect.Bool {
				return anyT
			}
		} else {
			return anyT
		}
	}
	return last
}

//	func ImplicitTypeConversionForPlus(vals ...*Value) reflect.Type {
//		resultType := GuessValuesTypeToBasicType(vals...)
//		if resultType.Kind() == literalReflectType_Interface.Kind() {
//			isString := true
//			for _, val := range vals {
//				if !val.IsString() && !val.IsInt() {
//					isString = false
//					break
//				}
//			}
//			if isString {
//				resultType = literalReflectType_String
//			}
//		}
//		return resultType
//	}
func GuessValuesKindToBasicType(vals ...*Value) reflect.Kind {
	return GuessValuesTypeToBasicType(vals...).Kind()
}

func (v *Frame) AutoConvertYakValueToNativeValue(val *Value) (reflect.Value, error) {
	i := (*interface{})(nil)

	if val.Value == nil {
		return reflect.ValueOf(i), nil
	}
	refV := reflect.ValueOf(val.Value)

	if val.IsYakFunction() {
		err := v.AutoConvertReflectValueByType(&refV, literalReflectType_NativeWarpFuntion)
		if err != nil {
			return reflect.Value{}, err
		}
		return refV, nil
	}
	refType := GuessValuesTypeToBasicType(val)
	err := v.AutoConvertReflectValueByType(&refV, refType)
	if err != nil {
		return reflect.Value{}, err
	}
	return refV, nil
}

func (v *Frame) AutoConvertReflectValueByType(
	reflectValue *reflect.Value,
	targetType /*, targetReflectType*/ reflect.Type,
) error {
	if reflectValue == nil || targetType == nil {
		return fmt.Errorf("invalid reflection conversion target or value")
	}
	srcKind := reflectValue.Kind()

	if srcKind == reflect.Invalid {
		*reflectValue = reflect.Zero(targetType) // work around `reflect: Call using zero Value argument`
		return nil
	}

	// 类型相同，不需要转换
	if targetType == reflectValue.Type() {
		return nil
	}

	targetKind := targetType.Kind()
	if targetKind == reflect.Interface {
		if !reflectValue.Type().AssignableTo(targetType) {
			return fmt.Errorf("invalid argument type: require `%v`, but we got `%v`", targetType, reflectValue.Type())
		}
		if targetType.NumMethod() > 0 {
			return nil
		}
		// 证明是别名，例如time.Duration 是 int64 类型别名，但是有自己实现的方法，所以不应该转换
		pkgPath := reflectValue.Type().PkgPath()
		if pkgPath != "" {
			return nil
		}
		//if targetReflectType != nil && yaklangspec.DontTyNormalize[targetReflectType] { // don't normalize input type
		//	return nil
		//}
		switch {
		case srcKind > reflect.Int && srcKind <= reflect.Int64:
			*reflectValue = reflect.ValueOf(int(reflectValue.Int()))
		case srcKind >= reflect.Uint && srcKind <= reflect.Uintptr:
			*reflectValue = reflect.ValueOf(int(reflectValue.Uint()))
		case srcKind == reflect.Float32:
			*reflectValue = reflect.ValueOf(reflectValue.Float())
		}

		return nil
	}

	srcType := reflectValue.Type()
	// Non-identical container types historically convert through a copy.
	if srcType.AssignableTo(targetType) && targetKind != reflect.Slice && targetKind != reflect.Array && targetKind != reflect.Map {
		return nil
	}

	switch targetKind {
	case reflect.Struct:
		if srcKind == reflect.Ptr {
			elemSrcType := srcType.Elem()
			if elemSrcType == targetType {
				if reflectValue.IsNil() {
					return fmt.Errorf("cannot dereference nil %v as %v", srcType, targetType)
				}
				*reflectValue = reflectValue.Elem()
				return nil
			}
		}
	case reflect.Func:
		if srcKind == reflect.Func && srcType.ConvertibleTo(targetType) {
			*reflectValue = reflectValue.Convert(targetType)
			return nil
		}
		if srcType == literalReflectType_YakFunction && !reflectValue.IsNil() {
			if v == nil {
				return utils.Errorf("cannot bind Yaklang.Function Calling for VirtualMachine!")
			}
			f := reflectValue.Interface().(*Function)
			// Snapshot the lexical frame on its owner goroutine. A retained or
			// asynchronous native callback must not read parent.scope while the
			// parent is exiting a block or restoring its defer scope.
			callbackParent := v
			callbackOwner := v.ownerGoroutineID
			synchronous := v.vm.config.synchronousExecution
			if !synchronous {
				callbackParent = NewSubFrame(v)
				callbackParent.ownerGoroutineID = callbackOwner
			}
			*reflectValue = reflect.MakeFunc(targetType, func(args []reflect.Value) []reflect.Value {
				var vmArgs []*Value
				// fix: unpack variadic args
				if targetType == literalReflectType_NativeWarpFuntion {
					newArgs, ok := args[0].Interface().([]interface{})
					if ok {
						vmArgs = make([]*Value, len(newArgs))
						for index, value := range newArgs {
							vmArgs[index] = NewAutoValue(value)
						}
					}
				}

				if vmArgs == nil {
					vmArgs = make([]*Value, len(args))
					for index, value := range args {
						vmArgs[index] = NewAutoValue(value.Interface())
					}
				}

				parent := callbackParent
				if synchronous || currentGoroutineID() == callbackOwner {
					parent = v
				}
				callbackFrame := parent.nativeCallbackFrame()
				result := callbackFrame.CallYakFunction(false, f, vmArgs)
				outCount := targetType.NumOut()
				if outCount <= 0 {
					return nil
				}
				reflectReturn := reflect.ValueOf(result)

				if outCount == 1 {
					expected := targetType.Out(0)
					err := callbackFrame.AutoConvertReflectValueByType(&reflectReturn, expected)
					if err != nil {
						panic(fmt.Sprintf("runtime error: cannot convert `%v` to `%v`: %v", reflectValueType(reflectReturn), expected, err))
					}
					return []reflect.Value{reflectReturn}
				}

				outputResults := make([]reflect.Value, outCount)
				if reflectReturn.Kind() != reflect.Slice || reflectReturn.Len() != outCount {
					panic(fmt.Sprintf("unexpected return value count, we need `%d` values", outCount))
				}
				for i := 0; i < outCount; i++ {
					val := reflectReturn.Index(i)
					if val.Kind() == reflect.Interface {
						val = val.Elem()
					}
					expectedType := targetType.Out(i)
					err := callbackFrame.AutoConvertReflectValueByType(&val, expectedType)
					if err != nil {
						panic(fmt.Sprintf("runtime error: cannot convert `%v` to `%v`: %v", reflectValueType(val), expectedType, err))
					}
					outputResults[i] = val
				}
				return outputResults
			})
			return nil
		} else {
			return utils.Errorf("cannot convert yaklang.Function to native calling...")
		}
	case reflect.Slice, reflect.Array: // 数组类型转换
		if srcKind == reflect.Slice || srcKind == reflect.Array {
			var resValRef reflect.Value
			if targetKind == reflect.Array {
				if targetType.Len() != reflectValue.Len() {
					return fmt.Errorf("cannot convert %v elements to %v", reflectValue.Len(), targetType)
				}
				resValRef = reflect.New(targetType).Elem()
			} else {
				resValRef = reflect.MakeSlice(targetType, reflectValue.Len(), reflectValue.Len())
			}
			reflectValueRef := reflect.ValueOf(reflectValue.Interface())
			elemType := targetType.Elem()
			for i := 0; i < reflectValueRef.Len(); i++ {
				val := reflectValueRef.Index(i)
				// unwrap interface element to get the underlying value
				if val.Kind() == reflect.Interface {
					if val.IsNil() {
						val = reflect.Zero(elemType)
					} else {
						val = val.Elem()
					}
				}
				err := v.AutoConvertReflectValueByType(&val, elemType)
				if err != nil {
					return err
				}
				resValRef.Index(i).Set(val)
			}
			*reflectValue = resValRef
			return nil
		}

	case reflect.Map:
		if srcKind == reflect.Map {
			resValRef := reflect.MakeMap(targetType)
			for _, key := range reflectValue.MapKeys() {
				val := reflectValue.MapIndex(key)
				err := v.AutoConvertReflectValueByType(&key, targetType.Key())
				if err != nil {
					return err
				}
				err = v.AutoConvertReflectValueByType(&val, targetType.Elem())
				if err != nil {
					return err
				}
				resValRef.SetMapIndex(key, val)
			}
			*reflectValue = resValRef
			return nil
		}
	default:
		if (targetKind == srcKind || convertible(srcKind, targetKind)) && srcType.ConvertibleTo(targetType) {
			*reflectValue = reflectValue.Convert(targetType)
			return nil
		}
	}
	// 2022.9.12 新增一些类型自动转换装置！
	//    1. 如果要求 []byte/[]uint8, 输入为 string 可以自动转换
	//    2. 如果要求为 string, 输入为 []byte / []uint8 也可以转
	if srcKind == reflect.String &&
		targetKind == reflect.Slice && targetType.Elem().Kind() == reflect.Uint8 {
		if srcType.ConvertibleTo(targetType) {
			*reflectValue = reflectValue.Convert(targetType)
			return nil
		}
	}
	if srcKind == reflect.Slice &&
		targetKind == reflect.String && (reflectValue.Type().Elem().Kind() == reflect.Uint8) {
		if srcType.ConvertibleTo(targetType) {
			*reflectValue = reflectValue.Convert(targetType)
			return nil
		}
	}

	// 2024.5.9 OrderedMap 与 map 相互转换
	// map -> orderedMap
	if srcKind == reflect.Map && targetType == literalReflectType_OrderedMap {
		*reflectValue = reflect.ValueOf(orderedmap.New(reflectValue.Interface()))
		return nil
	}
	// orderedMap -> map
	if srcType == literalReflectType_OrderedMap && targetKind == reflect.Map {
		v, ok := reflectValue.Interface().(*orderedmap.OrderedMap)
		targetMapKeyKind, targetMapValueKind := targetType.Key().Kind(), targetType.Elem().Kind()
		if ok && v != nil && targetMapValueKind == reflect.Interface {
			if targetMapKeyKind == reflect.String {
				*reflectValue = reflect.ValueOf(v.ToStringMap())
				return nil
			} else if targetMapKeyKind == reflect.Interface {
				*reflectValue = reflect.ValueOf(v.ToAnyMap())
				return nil
			}
		}
	}

	err := fmt.Errorf("invalid argument type: require `%v`, but we got `%v`", targetType, srcType)
	if strings.HasSuffix(fmt.Sprint(srcType), "spec.undefinedType") {
		err = fmt.Errorf("%v\n  Maybe u forgot to define variable?", err)
	}
	return err
}

func convertible(kind, tkind reflect.Kind) bool {
	if tkind >= reflect.Int && tkind <= reflect.Uintptr {
		return kind >= reflect.Int && kind <= reflect.Uintptr
	}
	if tkind == reflect.Float64 || tkind == reflect.Float32 {
		return kind >= reflect.Int && kind <= reflect.Float64
	}
	return false
}

func reflectValueType(value reflect.Value) interface{} {
	if !value.IsValid() {
		return "nil"
	}
	return value.Type()
}

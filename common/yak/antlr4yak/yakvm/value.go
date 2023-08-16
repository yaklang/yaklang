package yakvm

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Value struct {
	TypeVerbose string
	Value       interface{}
	Literal     string

	// 运行时才存在的成员

	// Identifier = expr
	// 当变量被赋值时，SymbolId 应该为赋值操作的根本
	SymbolId int
	// MemberCall 和 SliceCall 的 Caller 和 Collee
	// 一般来说 Caller.Callee 的时候，Callee 应该取 Identifier 的 Value 值
	// Caller[Callee] 的时候，Callee 取 Value
	// 这几个操作用反射都可以很容易做到
	CallerRef *Value
	CalleeRef *Value
	ExtraInfo map[string]interface{}
}

func (v *Value) AddExtraInfo(key string, info interface{}) {
	if v.ExtraInfo == nil {
		v.ExtraInfo = map[string]interface{}{}
	}
	v.ExtraInfo[key] = info
}

func (v *Value) GetExtraInfo(key string) interface{} {
	if v.ExtraInfo == nil {
		return nil
	}
	if val, ok := v.ExtraInfo[key]; ok {
		return val
	}
	return nil
}
func ChannelValueListToValue(op *Value) *Value {
	if !op.IsChannelValueList() {
		return op
	}
	op = NewAutoValue(op.CallSliceIndex(0))
	return op
}

func (v *Frame) getValueForLeftIterableCall(args []*Value) *Value {
	iterableValue := args[0]
	args = args[1:]

	argsLength := len(args)
	iterableValueType := reflect.TypeOf(iterableValue.Value)
	var iterableValueRF reflect.Value
	if iterableValueType.Kind() == reflect.String {
		if v, ok := iterableValue.Value.(string); ok {
			iterableValueRF = reflect.ValueOf([]rune(v))
		} else {
			panic("cannot convert string to []byte")
		}
	} else {
		iterableValueRF = reflect.ValueOf(iterableValue.Value)
	}
	if iterableValueRF.Type().Kind() == reflect.Ptr {
		iterableValueRF = iterableValueRF.Elem()
	}
	switch iterableValueRF.Type().Kind() {
	case reflect.String:
		fallthrough
	case reflect.Array, reflect.Slice:
		if argsLength != 1 {
			panic("left slice call args must be 1")
		}
		for _, arg := range args {
			if !arg.IsInt() {
				panic("slice call args must be int")
			}
		}
		start := args[0].Int()
		if start < 0 {
			start = iterableValueRF.Len() + start
		}
		if start < 0 || start >= iterableValueRF.Len() {
			panic("slice call error, start out of range")
		}

		var sliceRes reflect.Value

		// 这里可以转变为左值，因为 abc[expr] = xxx 是可以赋值的
		sliceRes = iterableValueRF.Index(start)
		if iterableValueType.Kind() == reflect.String {
			return NewValue("char", sliceRes.Interface().(rune), fmt.Sprintf("%c", sliceRes.Interface()))
		} else {
			value := NewValue(sliceRes.Type().String(), sliceRes.Interface(), fmt.Sprint(sliceRes))
			value.CallerRef = iterableValue
			value.CalleeRef = args[0]
			return value
		}
	case reflect.Map:
		if argsLength != 1 {
			panic("map call args must be 1")
		}
		mapRes := iterableValueRF.MapIndex(reflect.ValueOf(args[0].Value))
		if mapRes.IsValid() {
			return NewValue(mapRes.Type().String(), mapRes.Interface(), "")
		} else {
			return NewValue("nil", nil, "")
		}
	case reflect.Struct:
		if argsLength != 1 {
			panic("struct call args length must be 1")
		}
		if !args[0].IsStringOrBytes() {
			panic("struct call args must be string")
		}
		fieldName := args[0].Value.(string)
		memberValue := iterableValueRF.FieldByName(fieldName)
		if !memberValue.IsValid() {
			return undefined
		} else if !memberValue.CanInterface() {
			v.push(undefined)
			return undefined
		} else {
			return NewValue(memberValue.String(), memberValue.Interface(), fmt.Sprint(memberValue.Interface()))
		}
	default:
		panic(fmt.Sprintf("'%v' object is not subscriptable", iterableValueRF.Type().String()))
	}
	return undefined
}

// ConvertToLeftValue 当前值能不能转成左值？
// 这个是方法调用赋值的关键
func (v *Value) ConvertToLeftValue() (*Value, error) {
	selfConvertable := v.IsLeftValueRef() || v.IsLeftMemberCall() || v.IsLeftSliceCall()
	if selfConvertable {
		return v, nil
	}

	// 通过 Caller 和 Callee 记录路径，把右值可以专左值
	if v.CallerRef != nil && v.CalleeRef != nil {
		subVal := NewValue("memberCall.leftExpression", []*Value{v.CallerRef, v.CalleeRef}, "")
		switch {
		case subVal.IsLeftMemberCall():
			fallthrough
		case subVal.IsLeftSliceCall():
			return subVal, nil
		default:
			return nil, utils.Error("BUG: caller.callee or caller[callee] is not illegal")
		}
	}

	// 通过 CallerRef 记录符号可以转左值
	if v.CallerRef != nil && v.CallerRef.IsLeftValueRef() {
		return v.CallerRef, nil
	}

	return nil, utils.Error("cannot convert current value to left value")
}

func NewIntValue(i int) *Value {
	return &Value{
		TypeVerbose: "int",
		Value:       i,
		Literal:     fmt.Sprint(i),
	}
}

func NewInt64Value(i int64) *Value {
	return &Value{
		TypeVerbose: "int64",
		Value:       int(i),
		Literal:     fmt.Sprint(i),
	}
}

func NewBoolValue(b bool) *Value {
	return &Value{
		TypeVerbose: "bool",
		Value:       b,
		Literal:     fmt.Sprint(b),
	}
}

func NewAutoValue(b interface{}) *Value {
	if b == nil {
		return GetUndefined()
	}
	switch ret := b.(type) {
	case bool:
		return &Value{
			TypeVerbose: "bool",
			Value:       ret,
			Literal:     fmt.Sprint(b),
		}
	case int:
		return &Value{
			TypeVerbose: "int",
			Value:       ret,
			Literal:     fmt.Sprint(b),
		}
	case int8:
		return &Value{
			TypeVerbose: "int",
			Value:       int(ret),
			Literal:     fmt.Sprint(b),
		}
	case int16:
		return &Value{
			TypeVerbose: "int",
			Value:       int(ret),
			Literal:     fmt.Sprint(b),
		}
	case int32:
		return &Value{
			TypeVerbose: "int",
			Value:       int(ret),
			Literal:     fmt.Sprint(b),
		}
	case int64:
		return &Value{
			TypeVerbose: "int64",
			Value:       int64(ret),
			Literal:     fmt.Sprint(b),
		}
	case uint8: // byte
		return &Value{
			TypeVerbose: "byte",
			Value:       byte(ret),
			Literal:     fmt.Sprint(b),
		}
	case uint:
		return &Value{
			TypeVerbose: "int",
			Value:       int(ret),
			Literal:     fmt.Sprint(b),
		}
	case uint16:
		return &Value{
			TypeVerbose: "int",
			Value:       int(ret),
			Literal:     fmt.Sprint(b),
		}
	case uint32:
		return &Value{
			TypeVerbose: "int",
			Value:       int(ret),
			Literal:     fmt.Sprint(b),
		}
	case uint64:
		return &Value{
			TypeVerbose: "int64",
			Value:       int64(ret),
			Literal:     fmt.Sprint(b),
		}
	case float64:
		return &Value{
			TypeVerbose: "float64",
			Value:       ret,
			Literal:     fmt.Sprint(b),
		}
	case float32:
		return &Value{
			TypeVerbose: "float64",
			Value:       float64(ret),
			Literal:     fmt.Sprint(b),
		}
	case []byte:
		return &Value{
			TypeVerbose: "[]byte",
			Value:       ret,
			Literal:     fmt.Sprint(b),
		}
	default:
		return &Value{
			TypeVerbose: reflect.TypeOf(b).String(),
			Value:       b,
			Literal:     fmt.Sprint(b),
		}
	}
}

func NewStringValue(i string) *Value {
	return &Value{
		TypeVerbose: "string",
		Value:       i,
		Literal:     strconv.Quote(i),
	}
}

func NewEmptyMap(lit string) *Value {
	return &Value{
		TypeVerbose: "map[string]interface{}",
		Value:       make(map[string]interface{}),
		Literal:     lit,
	}
}

func NewGenericMap(lit string) *Value {
	return &Value{
		TypeVerbose: "map[interface{}]interface{}",
		Value:       make(map[interface{}]interface{}),
		Literal:     lit,
	}
}

func NewValue(typeStr string, value interface{}, lit string) *Value {
	return &Value{
		TypeVerbose: typeStr,
		Value:       value,
		Literal:     lit,
	}
}

func NewType(typeStr string, value reflect.Type) *Value {
	return &Value{
		TypeVerbose: typeStr,
		Value:       value,
		Literal:     "",
	}
}

func NewGeneralMap(lit string) *Value {
	return &Value{
		TypeVerbose: "map[string]interface{}",
		Value:       make(map[string]interface{}),
		Literal:     lit,
	}
}

func NewStringSliceValue(i []string) *Value {
	return &Value{
		TypeVerbose: "[]string",
		Value:       i,
		Literal:     fmt.Sprintf("%v", i),
	}
}

func (v *Value) Type() reflect.Type {
	t, ok := v.Value.(reflect.Type)
	if ok {
		return t
	}
	return nil
}

func (v *Value) TypeStr() string {
	if v.IsType() {
		return "type"
	}
	return reflect.TypeOf(v.Value).String()
}

func (v *Value) IsType() bool {
	_, ok := v.Value.(reflect.Type)
	return ok
}

func (v *Value) IsChannel() bool {
	t := reflect.TypeOf(v.Value).Kind()
	return t == reflect.Chan
}

func (v *Value) IsMap() bool {
	t := reflect.TypeOf(v.Value).Kind()
	return t == reflect.Map
}

func (v *Value) Rangeable() bool {
	rk := reflect.TypeOf(v.Value).Kind()
	return rk == reflect.Slice || rk == reflect.Array || rk == reflect.Map || rk == reflect.Chan || v.IsInt64()
}

func (v *Value) GetIndexedVariableCount() int {
	return GetIndexedVariableCount(v.Value)
}

func (v *Value) GetNamedVariableCount() int {
	return GetNamedVariableCount(v.Value)
}

func (v *Value) Callable() bool {
	return v.NativeCallable() || v.IsYakFunction()
}

func (v *Value) IsYakFunction() bool {
	_, ok := v.Value.(*Function)
	return ok
}

func (v *Value) NativeCallable() bool {
	if v == nil {
		return false
	}
	if v.Value == nil {
		return false
	}
	kind := reflect.TypeOf(v.Value).Kind()
	return kind == reflect.Func
}

func (v *Value) AsString() string {
	return AsString(v.Value)
}

func (v *Value) IsBytesOrRunes() bool {
	return IsBytesOrRunes(v.Value)
}

func (v *Value) IsStringOrBytes() bool {
	_, ok := v.Value.(string)
	if !ok {
		_, ok := v.Value.([]byte)
		return ok
	}
	return ok
}

func (v *Value) IsString() bool {
	_, ok := v.Value.(string)
	return ok
}

func (v *Value) String() string {
	if v == nil {
		return "-"
	}

	if v.IsStringOrBytes() {
		return v.AsString()
	}

	if v.IsYakFunction() {
		return v.Value.(*Function).String()
	}

	//if v.Literal != "" {
	//	return v.Literal
	//}

	return fmt.Sprintf("%v", v.Value)
	//if v.TypeVerbose != "" {
	//	return fmt.Sprintf("%#v.(%v)", v.Value, v.TypeVerbose)
	//} else {
	//	return fmt.Sprintf("%#v", v.Value)
	//}
}

const _IdentifierValueType = `__identifier__`

func NewIdentifierValue(i string) *Value {
	return &Value{
		TypeVerbose: _IdentifierValueType,
		Value:       i,
		Literal:     i,
	}
}

func (v *Value) IsIdentifier() bool {
	return v.TypeVerbose == _IdentifierValueType
}

func (v *Value) IsByte() bool {
	switch v.Value.(type) {
	case uint8:
		return true
	}
	return false
}

func (v *Value) IsFloat() bool {
	switch v.Value.(type) {
	case float64, float32:
		return true
	}
	return false
}

func (v *Value) IsBool() bool {
	switch v.Value.(type) {
	case bool:
		return true
	}
	return false
}

func (v *Value) IsBytes() bool {
	_, ok := v.Value.([]byte)
	return ok
}

func (v *Value) Bytes() []byte {
	b, ok := v.Value.([]byte)
	if ok {
		return b
	}
	return nil
}

func (v *Value) IsUndefined() bool {
	if v == nil {
		return true
	}

	if v.Value == nil {
		return true
	}

	if v.TypeVerbose == "undefined" && v.Literal == "undefined" {
		return true
	}

	return false
}

func (v *Value) Bool() bool {
	switch b := v.Value.(type) {
	case bool:
		return b
	}
	return false
}

func (v *Value) IsCodes() bool {
	_, ok := v.Value.([]*Code)
	return ok
}

func (v *Value) IsValueList() bool {
	_, ok := v.Value.([]*Value)
	return ok
}

func (v *Value) IsChannelValueList() bool {
	return v.TypeVerbose == "__channel__opcode_list__"
}

func (v *Value) ValueListToInterface() interface{} {
	list := v.ValueList()
	listLen := len(list)
	switch listLen {
	case 1:
		return list[0].Value
	case 0:
		return nil
	default:
		var ret = make([]interface{}, listLen)
		for i := 0; i < listLen; i++ {
			ret[i] = list[i].Value
		}
		return ret
	}
}

func (v *Value) Codes() []*Code {
	codes, _ := v.Value.([]*Code)
	return codes
}

func (v *Value) ValueList() []*Value {
	i, _ := v.Value.([]*Value)
	return i
}

func (v *Value) AssignBySymbol(table *Scope, val *Value) {
	if v == nil && !v.IsLeftValueRef() {
		panic("assign failed, must assign to yakvm.IsLeftValueRef()")
	}

	current := table
	for {
		if current == nil {
			// 找不到符号对应的值的 scope
			table.NewValueByID(v.SymbolId, val)
			break
		}

		if current.InCurrentScope(v.SymbolId) {
			current.NewValueByID(v.SymbolId, val)
			break
		} else {
			current = current.parent
		}
	}
	val.CallerRef = v
}

func (v *Value) GlobalAssignBySymbol(table *Scope, val *Value) {
	if v == nil && !v.IsLeftValueRef() {
		panic("global assign failed, must assign to yakvm.IsLeftValueRef()")
	}

	current := table
	for {
		if current.parent == nil {
			// 找不到符号对应的值的 scope
			current.NewValueByID(v.SymbolId, val)
			break
		}

		if current.InCurrentScope(v.SymbolId) {
			current.NewValueByID(v.SymbolId, val)
			break
		} else {
			current = current.parent
		}
	}
	val.CallerRef = v
}

func (v *Value) IsIterable() bool {
	if v == nil {
		return false
	}
	rk := reflect.TypeOf(v.Value).Kind()
	return rk == reflect.Slice || rk == reflect.Array
}

func (v *Value) CallSliceIndex(i int) interface{} {
	return reflect.ValueOf(v.Value).Index(i).Interface()
}

func (v *Value) Len() int {
	return reflect.ValueOf(v.Value).Len()
}

func (v *Value) True() bool {
	return !v.False()
}

func (v *Value) LuaTrue() bool {
	return !v.LuaFalse()
}

func (v *Value) False() bool {
	if v == nil || v.Value == nil {
		return true
	}

	if v == undefined {
		return true
	}

	if v.IsUndefined() {
		return true
	}

	b, ok := v.Value.(bool)
	if ok {
		return !b
	}

	return funk.IsEmpty(v.Value)
}

func (v *Value) LuaFalse() bool {
	if v == nil || v.Value == nil {
		return true
	}

	if v == undefined {
		return true
	}

	if v.IsUndefined() {
		return true
	}

	b, ok := v.Value.(bool)
	if ok {
		return !b
	}

	if zero, ok := v.Value.(int); ok {
		if zero == 0 {
			return false
		}
	}

	return funk.IsEmpty(v.Value)
}

func (v *Value) Float64() float64 {
	switch ret := v.Value.(type) {
	case float64:
		return ret
	case float32:
		return float64(ret)
	}

	if v.IsInt64() {
		return float64(v.Int64())
	}
	return float64(0)
}

func (v *Value) IsInt64() bool {
	switch v.Value.(type) {
	case int, int64, int8, int16, int32,
		uint, uint8, uint16, uint32, uint64:
		return true
	}
	return false
}

func (v *Value) IsInt64EX() (int64, bool) {
	switch ret := v.Value.(type) {
	case int:
		return int64(ret), true
	case int64:
		return ret, true
	case int8:
		return int64(ret), true
	case int16:
		return int64(ret), true
	case int32:
		return int64(ret), true
	case uint:
		return int64(ret), true
	case uint8:
		return int64(ret), true
	case uint16:
		return int64(ret), true
	case uint32:
		return int64(ret), true
	case uint64:
		log.Errorf("uint64 to int64 overflow, value: %d", ret)
		return int64(ret), true
	}
	return 0, false
}

func (v *Value) IsInt() bool {
	switch v.Value.(type) {
	case int, int64, int8, int16, int32,
		uint, uint8, uint16, uint32, uint64:
		return true
	}
	return false
}

func (v *Value) Int() int {
	if v == nil {
		return 0
	}
	if v.Value == nil {
		return 0
	}
	switch ret := v.Value.(type) {
	case int:
		return ret
	case int8:
		return int(ret)
	case int16:
		return int(ret)
	case int32:
		return int(ret)
	case int64:
		return int(ret)
	case uint:
		return int(ret)
	case uint8:
		return int(ret)
	case uint16:
		return int(ret)
	case uint32:
		return int(ret)
	case uint64:
		return int(ret)
	}
	return 0
}

func (v *Value) Int64() int64 {
	switch ret := v.Value.(type) {
	case int:
		return int64(ret)
	case int8:
		return int64(ret)
	case int16:
		return int64(ret)
	case int32:
		return int64(ret)
	case int64:
		return ret
	case uint:
		return int64(ret)
	case uint8:
		return int64(ret)
	case uint16:
		return int64(ret)
	case uint32:
		return int64(ret)
	case uint64:
		return int64(ret)
	}
	return 0
}

func NewValueRef(id int) *Value {
	return &Value{
		TypeVerbose: "ref",
		Literal:     `__symbol_` + fmt.Sprint(id) + `__`,
		SymbolId:    id,
	}
}

func (v *Value) IsLeftValueRef() bool {
	if v == nil {
		return false
	}
	return v.SymbolId > 0
}

func (v *Value) IsLeftSliceCall() bool {
	if v == nil || v.Value == nil {
		return false
	}
	raw, ok := v.Value.([]*Value)
	if !ok {
		return false
	}

	if len(raw) != 2 {
		return false
	}

	// x[...] 的调用仅限于 slice / map / string

	switch reflect.TypeOf(raw[0].Value).Kind() {
	case reflect.Slice, reflect.String, reflect.Map:
		return true
	default:
		return false
	}
}

var (
	leftSliceAssignLock = new(sync.Mutex)
)

func (v *Value) LeftSliceAssignTo(vir *Frame, val *Value) {
	if !v.IsLeftSliceCall() {
		return
	}
	caller, key := v.GetLeftCallerNIndex()
	leftSliceAssignLock.Lock()
	defer leftSliceAssignLock.Unlock()
	switch reflect.TypeOf(caller.Value).Kind() {
	case reflect.Slice:
		reflect.ValueOf(caller.Value).Index(key.Int()).Set(reflect.ValueOf(val.Value).Convert(reflect.ValueOf(caller.Value).Index(key.Int()).Type()))
	case reflect.String:
		if !val.IsByte() {
			panic("runtime error: cannot assign %v to string[index]")
		}
		panic("BUG: not implemented for string[...]")
	case reflect.Map:
		refV, err := vir.AutoConvertYakValueToNativeValue(val)
		if err != nil {
			panic(fmt.Sprintf("runtime error: cannot assign %v to map[index]", val))
		}
		callerRefV := reflect.ValueOf(caller.Value)
		keyRefV := reflect.ValueOf(key.Value)
		valueRefV := callerRefV.MapIndex(keyRefV)
		if valueRefV.IsValid() {
			if refV.CanConvert(valueRefV.Type()) {
				refV = refV.Convert(valueRefV.Type())
			} else {
				panic(fmt.Sprintf("runtime error: cannot convert %v to %v", val, valueRefV.Type()))
			}
		}
		callerRefV.SetMapIndex(keyRefV, refV)
	default:
		panic("*yakvm.Value.LeftSliceAssignTo not implemented")
	}
}

func (v *Value) LuaLeftSliceAssignTo(vir *Frame, val *Value) {
	if !v.IsLeftSliceCall() {
		return
	}
	caller, key := v.GetLeftCallerNIndex()
	leftSliceAssignLock.Lock()
	defer leftSliceAssignLock.Unlock()
	switch reflect.TypeOf(caller.Value).Kind() {
	case reflect.Slice:
		reflect.ValueOf(caller.Value).Index(key.Int()).Set(reflect.ValueOf(val.Value))
	case reflect.String:
		if !val.IsByte() {
			panic("runtime error: cannot assign %v to string[index]")
		}
		panic("BUG: not implemented for string[...]")
	case reflect.Map:
		refV := reflect.ValueOf(val.Value)
		reflect.ValueOf(caller.Value).SetMapIndex(reflect.ValueOf(key.Value), refV)

	default:
		panic("*yakvm.Value.LeftSliceAssignTo not implemented")
	}
}

func (v *Value) GetLeftCallerNIndex() (*Value, *Value) {
	if v.IsLeftSliceCall() || v.IsLeftMemberCall() {
		raw := v.Value.([]*Value)
		return raw[0], raw[1]
	}
	return nil, nil
}

func (v *Value) IsLeftMemberCall() bool {
	if v == nil || v.Value == nil {
		return false
	}
	raw, ok := v.Value.([]*Value)
	if !ok {
		return false
	}

	if len(raw) != 2 {
		return false
	}

	if reflect.TypeOf(raw[1].Value).Kind() != reflect.String {
		return false
	}
	return true
}

func (v *Value) LeftMemberAssignTo(vir *Frame, val *Value) {
	if !v.IsLeftMemberCall() {
		return
	}
	caller, key := v.GetLeftCallerNIndex()
	callerValue := caller.Value
	switch reflect.TypeOf(caller.Value).Kind() {
	case reflect.Map:
		refV, err := vir.AutoConvertYakValueToNativeValue(val)
		if err != nil {
			panic(fmt.Sprintf("runtime error: cannot assign %v to map[index]", val))
		}
		callerRefV := reflect.ValueOf(caller.Value)
		keyRefV := reflect.ValueOf(key.Value)
		if callerRefV.MapIndex(keyRefV).IsValid() {
			refV = refV.Convert(callerRefV.MapIndex(keyRefV).Type())
		}
		callerRefV.SetMapIndex(keyRefV, refV)
	case reflect.Struct:
		log.Warnf("Cannot assign to struct field %s", key.AsString())
	case reflect.Ptr:
		keyValue := key.AsString()
		structRefv := reflect.ValueOf(callerValue).Elem().FieldByName(keyValue)
		refV := reflect.ValueOf(val.Value)
		err := vir.AutoConvertReflectValueByType(&refV, refV.Type())
		if err != nil {
			panic(fmt.Sprintf("not support type %v", refV.Type()))
		}
		structRefv.Set(refV.Convert(structRefv.Type()))
	default:
		panic(fmt.Sprintf("not implemented for + %v[%v]", reflect.TypeOf(caller), reflect.TypeOf(caller)))
	}
}

func (_v *Value) Equal(value *Value) bool {
	if _v.IsInt() && value.IsInt() {
		return _v.Int() == value.Int()
	}

	if _v.IsFloat() && value.IsFloat() {
		return _v.Float64() == value.Float64()
	}

	if _v.IsFloat() && value.IsInt() {
		return _v.Float64() == value.Float64()
	}

	if value.IsFloat() && _v.IsInt() {
		return _v.Float64() == value.Float64()
	}

	if value.IsBool() && value.IsBool() {
		return _v.True() == value.True()
	}

	if value.IsBytes() || _v.IsBytes() {
		// 如果任意一个是 bytes 的话，都转为 string 进行比较
		return _v.String() == value.String()
	}

	if ret, ok := _v.IsInt64EX(); value.IsStringOrBytes() && ok {
		if targetVal := []rune(value.AsString()); len(targetVal) == 1 {
			return ret == int64(targetVal[0])
		}
	}

	if ret, ok := value.IsInt64EX(); _v.IsStringOrBytes() && ok {
		if targetVal := []rune(_v.AsString()); len(targetVal) == 1 {
			return ret == int64(targetVal[0])
		}
	}

	// 如果任意又一个值为 undefined 的话
	if _v.IsUndefined() || value.IsUndefined() {
		return _v.False() == value.False()
	}

	return funk.Equal(_v.Value, value.Value)
}

func (_v *Value) Assign(vir *Frame, right *Value) {
	left, err := _v.ConvertToLeftValue()
	if err != nil {
		panic("BUG: assign failed: " + err.Error())
	}

	switch true {
	case left.IsLeftValueRef():
		var val interface{}
		if right.IsChannelValueList() {
			val = right.CallSliceIndex(0)
		} else {
			val = right.Value
		}
		left.AssignBySymbol(vir.CurrentScope(), NewAutoValue(val))
		return
	case left.IsLeftSliceCall():
		left.LeftSliceAssignTo(vir, right)
	case left.IsLeftMemberCall():
		left.LeftMemberAssignTo(vir, right)
	default:
		panic("runtime error: cannot assign left `" + reflect.TypeOf(left.Value).String() + "`")
	}
}

func (_v *Value) GlobalAssign(vir *Frame, right *Value) {
	left, err := _v.ConvertToLeftValue()
	if err != nil {
		panic("BUG: assign failed: " + err.Error())
	}

	switch true {
	case left.IsLeftValueRef():
		var val interface{}
		if right.IsChannelValueList() {
			val = right.CallSliceIndex(0)
		} else {
			val = right.Value
		}
		left.GlobalAssignBySymbol(vir.CurrentScope(), NewAutoValue(val))
		return
	case left.IsLeftSliceCall():
		left.LuaLeftSliceAssignTo(vir, right) // TODO: 这里还要仔细看一下 和yak目前不太一样
	case left.IsLeftMemberCall():
		left.LeftMemberAssignTo(vir, right)
	default:
		panic("runtime error: cannot assign left `" + reflect.TypeOf(left.Value).String() + "`")
	}
}

func NewValues(val []*Value) *Value {
	vars := make([]interface{}, len(val))
	for index, i := range val {
		vars[index] = i.Value
	}
	return &Value{
		TypeVerbose: "[]interface{}",
		Value:       vars,
	}
}

func GetIndexedVariableCount(v interface{}) int {
	rv := reflect.ValueOf(v)
	rk := rv.Kind()
	if rk == reflect.Ptr {
		rv = rv.Elem()
		rk = rv.Kind()
	}
	ok := rk == reflect.Slice || rk == reflect.Array || rk == reflect.Chan || rk == reflect.Map
	if !ok {
		return 0
	}

	return rv.Len()
}

func GetNamedVariableCount(v interface{}) int {
	rv := reflect.ValueOf(v)
	rk := rv.Kind()
	if rk == reflect.Ptr {
		rv = rv.Elem()
		rk = rv.Kind()
	}
	if _, ok := v.(*Function); rk == reflect.Struct && !ok {
		return rv.NumField()
	} else if rk == reflect.Map && rv.Len() > 0 {
		// len()
		return 1
	} else if IsBytesOrRunes(v) {
		// string()
		return 1
	}
	return 0
}

func IsBytesOrRunes(v interface{}) bool {
	_, ok := v.([]byte)
	if !ok {
		_, ok := v.([]rune)
		return ok
	}
	return ok
}

func AsString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case []rune:
		return string(s)
	case *Function:
		return s.String()
	default:
		return fmt.Sprintf("%#v", s)
	}
}

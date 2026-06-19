package yserx

import (
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"math"
)

type JavaFieldValue struct {
	Type        byte   `json:"type"`
	TypeVerbose string `json:"type_verbose"`

	FieldType        byte   `json:"field_type"`
	FieldTypeVerbose string `json:"field_type_verbose"`

	Bytes []byte `json:"bytes,omitempty"`

	// for array / object
	Object JavaSerializable `json:"object,omitempty"`
}

func (v *JavaFieldValue) Marshal(cfg *MarshalContext) []byte {
	var raw []byte
	//raw = append(raw, v.FieldType)
	if v.FieldType == JT_OBJECT || v.FieldType == JT_ARRAY {
		return append(raw, v.Object.Marshal(cfg)...)
	}
	raw = append(raw, v.Bytes...)
	return raw
}

// NewJavaFieldValue 创建一个指定类型与原始字节的 Java 序列化字段值，是各类字段值构造的基础函数
// 在 yak 中通过 java.NewJavaFieldValue 调用
// 参数:
//   - t: 字段类型标记(如 JT_INT、JT_BYTE 等)
//   - raw: 字段值的原始字节
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造一个字节型字段值
// v = java.NewJavaFieldValue(0x42, []byte{0x01})
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldValue(t byte, raw []byte) *JavaFieldValue {
	v := &JavaFieldValue{
		FieldType:        t,
		FieldTypeVerbose: jtToVerbose(t),
		Bytes:            raw,
	}
	initTCType(v)
	return v
}

// NewJavaFieldByteValue 创建一个 byte 类型的 Java 序列化字段值
// 在 yak 中通过 java.NewJavaFieldByteValue 调用
// 参数:
//   - b: 字节值
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造 byte 字段值
// v = java.NewJavaFieldByteValue(0x41)
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldByteValue(b byte) *JavaFieldValue {
	return NewJavaFieldValue(JT_BYTE, []byte{b})
}

// NewJavaFieldBoolValue 创建一个 boolean 类型的 Java 序列化字段值
// 在 yak 中通过 java.NewJavaFieldBoolValue 调用
// 参数:
//   - b: 布尔值
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造 bool 字段值
// v = java.NewJavaFieldBoolValue(true)
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldBoolValue(b bool) *JavaFieldValue {
	var raw []byte
	if b {
		raw = append(raw, 1)
	} else {
		raw = append(raw, 0)
	}
	return NewJavaFieldValue(JT_BOOL, raw)
}

// NewJavaFieldShortValue 创建一个 short 类型的 Java 序列化字段值
// 在 yak 中通过 java.NewJavaFieldShortValue 调用
// 参数:
//   - i: short 整数值
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造 short 字段值
// v = java.NewJavaFieldShortValue(1024)
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldShortValue(i int) *JavaFieldValue {
	return NewJavaFieldValue(JT_SHORT, IntTo2Bytes(i))
}

// NewJavaFieldCharValue 创建一个 char 类型的 Java 序列化字段值
// 在 yak 中通过 java.NewJavaFieldCharValue 调用
// 参数:
//   - i: char 对应的整数码点
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造 char 字段值
// v = java.NewJavaFieldCharValue(65)
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldCharValue(i int) *JavaFieldValue {
	return NewJavaFieldValue(JT_CHAR, IntTo2Bytes(i))
}

// NewJavaFieldIntValue 创建一个 int 类型的 Java 序列化字段值
// 在 yak 中通过 java.NewJavaFieldIntValue 调用
// 参数:
//   - i: int 整数值
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造 int 字段值
// v = java.NewJavaFieldIntValue(123456)
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldIntValue(i uint64) *JavaFieldValue {
	return NewJavaFieldValue(JT_INT, Uint64To4Bytes(i))
}

// NewJavaFieldLongValue 创建一个 long 类型的 Java 序列化字段值
// 在 yak 中通过 java.NewJavaFieldLongValue 调用
// 参数:
//   - i: long 整数值
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造 long 字段值
// v = java.NewJavaFieldLongValue(123456789)
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldLongValue(i uint64) *JavaFieldValue {
	return NewJavaFieldValue(JT_LONG, Uint64To8Bytes(i))
}

// NewJavaFieldFloatValue 创建一个 float 类型的 Java 序列化字段值
// 在 yak 中通过 java.NewJavaFieldFloatValue 调用
// 参数:
//   - i: float 浮点值
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造 float 字段值
// v = java.NewJavaFieldFloatValue(1.5)
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldFloatValue(i float32) *JavaFieldValue {
	return NewJavaFieldValue(JT_FLOAT, Uint64To4Bytes(uint64(math.Float32bits(i))))
}

// NewJavaFieldDoubleValue 创建一个 double 类型的 Java 序列化字段值
// 在 yak 中通过 java.NewJavaFieldDoubleValue 调用
// 参数:
//   - i: double 浮点值
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造 double 字段值
// v = java.NewJavaFieldDoubleValue(3.14159)
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldDoubleValue(i float64) *JavaFieldValue {
	return NewJavaFieldValue(JT_DOUBLE, Uint64To8Bytes(uint64(math.Float64bits(i))))
}

// NewJavaFieldArrayValue 创建一个数组类型的 Java 序列化字段值，承载数组/null/引用对象
// 在 yak 中通过 java.NewJavaFieldArrayValue 调用
// 参数:
//   - i: 数组、null 或引用类型的 Java 序列化对象
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造数组字段值
// v = java.NewJavaFieldArrayValue(java.NewJavaNull())
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldArrayValue(i JavaSerializable) *JavaFieldValue {
	v := &JavaFieldValue{
		FieldType:        JT_ARRAY,
		FieldTypeVerbose: jtToVerbose(JT_ARRAY),
	}
	switch ret := i.(type) {
	case *JavaNull:
		v.Object = ret
		initTCType(v)
		return v
	case *JavaArray:
		v.Object = ret
		initTCType(v)
		return v
	case *JavaReference:
		v.Object = ret
		initTCType(v)
		return v
	}
	return v
}

// NewJavaFieldObjectValue 创建一个对象类型的 Java 序列化字段值，承载对象/字符串/类等引用类型
// 在 yak 中通过 java.NewJavaFieldObjectValue 调用
// 参数:
//   - i: 对象、字符串、类、数组、枚举、引用或 null 类型的 Java 序列化对象
//
// 返回值:
//   - Java 字段值序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造对象字段值
// v = java.NewJavaFieldObjectValue(java.NewJavaString("hello"))
// println(v.FieldTypeVerbose)
// ```
func NewJavaFieldObjectValue(i JavaSerializable) *JavaFieldValue {
	v := &JavaFieldValue{
		FieldType:        JT_OBJECT,
		FieldTypeVerbose: jtToVerbose(JT_OBJECT),
	}
	switch i.(type) {
	case *JavaObject:
	case *JavaReference:
	case *JavaNull:
	case *JavaString:
	case *JavaClass:
	case *JavaArray:
	case *JavaEnumDesc:
	default:
		return v
	}

	v.Object = i
	initTCType(v)
	return v
}

func NewJavaFieldBytes(rawStr string) *JavaFieldValue {
	var vals []*JavaFieldValue
	for _, b := range []byte(rawStr) {
		vals = append(vals, NewJavaFieldByteValue(b))
	}

	raw, _ := codec.DecodeBase64("rPMX+AYIVOA=")
	return NewJavaFieldArrayValue(NewJavaArray(
		NewJavaClassDesc(
			"[B",
			raw,
			2, NewJavaClassFields(), nil, nil,
		),
		vals...))
}

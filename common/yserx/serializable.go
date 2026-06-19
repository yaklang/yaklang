package yserx

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/ioutil"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type JavaSerializable interface {
	//String() string
	//SDumper(indent int) string
	Marshal(*MarshalContext) []byte
}

// ParseJavaObjectStream 解析 Java 序列化字节流，返回其中包含的 Java 序列化对象列表
// 在 yak 中通过 java.ParseJavaObjectStream 调用，是 java.MarshalJavaObjects 的逆操作
// 参数:
//   - raw: Java 序列化字节流
//
// 返回值:
//   - 解析得到的 Java 序列化对象列表
//   - 错误信息，失败时非 nil
//
// Example:
// ```
// s = java.NewJavaString("hello")
// b = java.MarshalJavaObjects(s)
// objs = java.ParseJavaObjectStream(b)~
// assert len(objs) == 1, "should parse exactly one object"
// ```
func ParseJavaSerialized(raw []byte) ([]JavaSerializable, error) {
	r := bufio.NewReader(bytes.NewBuffer(raw))
	return ParseJavaSerializedEx(r, ioutil.Discard)
}

// ParseHexJavaObjectStream 解析十六进制编码的 Java 序列化字节流，返回 Java 序列化对象列表
// 在 yak 中通过 java.ParseHexJavaObjectStream 调用，适用于已被 hex 编码的序列化数据
// 参数:
//   - raw: 十六进制编码的 Java 序列化字节流
//
// 返回值:
//   - 解析得到的 Java 序列化对象列表
//   - 错误信息，失败时非 nil
//
// Example:
// ```
// s = java.NewJavaString("hello")
// h = codec.EncodeToHex(java.MarshalJavaObjects(s))
// objs = java.ParseHexJavaObjectStream(h)~
// assert len(objs) == 1, "should parse exactly one object"
// ```
func ParseHexJavaSerialized(raw string) ([]JavaSerializable, error) {
	decoded, err := codec.DecodeHex(raw)
	if err != nil {
		return nil, err
	}
	return ParseJavaSerialized(decoded)
}

// FromJson 将 java.ToJson 生成的 JSON 还原为 Java 序列化对象列表
// 在 yak 中通过 java.FromJson 调用，是 java.ToJson 的逆操作
// 参数:
//   - raw: java.ToJson 生成的 JSON 字节数组
//
// 返回值:
//   - 还原出的 Java 序列化对象列表
//   - 错误信息，失败时非 nil
//
// Example:
// ```
// s = java.NewJavaString("hello")
// b = java.MarshalJavaObjects(s)
// objs = java.ParseJavaObjectStream(b)~
// j = java.ToJson(objs)~
// restored = java.FromJson(j)~
// assert len(restored) == len(objs), "restored object count should match"
// ```
func FromJson(raw []byte) ([]JavaSerializable, error) {
	var objs []json.RawMessage
	_ = json.Unmarshal(raw, &objs)
	if len(objs) > 0 {
		var serls []JavaSerializable
		for _, raw := range objs {
			o, err := _rawIdentToJavaSerializable(raw)
			if err != nil {
				return nil, err
			}
			initTCType(o)
			serls = append(serls, o)
		}
		return serls, nil
	}

	o, err := _rawIdentToJavaSerializable(raw)
	if err != nil {
		return nil, err
	}
	initTCType(o)
	return []JavaSerializable{o}, nil
}

type JavaMarshaler interface {
	ObjectMarshaler(obj *JavaObject, ctx *MarshalContext) []byte
	ClassDescMarshaler(obj *JavaClassDetails, ctx *MarshalContext) []byte
}
type CommonMarshaler struct {
}

func (j *CommonMarshaler) ObjectMarshaler(obj *JavaObject, ctx *MarshalContext) []byte {
	var raw = []byte{TC_OBJECT}

	raw = append(raw, obj.Class.Marshal(ctx)...)
	for _, i := range obj.ClassData {
		raw = append(raw, i.Marshal(ctx)...)
	}
	return raw
}
func (j *CommonMarshaler) ClassDescMarshaler(obj *JavaClassDetails, ctx *MarshalContext) []byte {
	nullRaw := []byte{TC_NULL}
	if obj == nil {
		return nullRaw
	}

	if obj.IsNull {
		return nullRaw
	}

	if obj.DynamicProxyClass {
		raw := []byte{TC_PROXYCLASSDESC}
		raw = append(raw, IntTo4Bytes(obj.DynamicProxyClassInterfaceCount)...)
		for _, i := range obj.DynamicProxyClassInterfaceNames {
			raw = append(raw, marshalString(i, ctx.StringCharLength)...)
		}
		for _, i := range obj.DynamicProxyAnnotation {
			raw = append(raw, i.Marshal(ctx)...)
		}
		raw = append(raw, TC_ENDBLOCKDATA)
		if obj.SuperClass == nil {
			return raw
		} else {
			raw = append(raw, obj.SuperClass.Marshal(ctx)...)
			return raw
		}
	}

	raw := []byte{TC_CLASSDESC}
	raw = append(raw, marshalString(obj.ClassName, ctx.StringCharLength)...)
	raw = append(raw, obj.SerialVersion...)
	raw = append(raw, obj.DescFlag)
	raw = append(raw, obj.Fields.Marshal(ctx)...)

	// annotation
	for _, i := range obj.Annotations {
		raw = append(raw, i.Marshal(ctx)...)
	}
	raw = append(raw, TC_ENDBLOCKDATA)

	if obj.SuperClass == nil {
		raw = append(raw, TC_NULL)
	} else {
		raw = append(raw, obj.SuperClass.Marshal(ctx)...)
	}
	return raw
}

type JRMPMarshaler struct {
	CodeBase  string
	descTimes int
	*CommonMarshaler
}

func (j *JRMPMarshaler) ClassDescMarshaler(obj *JavaClassDetails, ctx *MarshalContext) []byte {
	defer func() {
		j.descTimes++
	}()
	nullRaw := []byte{TC_NULL}
	if obj == nil {
		return nullRaw
	}

	if obj.IsNull {
		return nullRaw
	}

	if obj.DynamicProxyClass {
		raw := []byte{TC_PROXYCLASSDESC}
		raw = append(raw, IntTo4Bytes(obj.DynamicProxyClassInterfaceCount)...)
		for _, i := range obj.DynamicProxyClassInterfaceNames {
			raw = append(raw, marshalString(i, ctx.StringCharLength)...)
		}
		for _, i := range obj.DynamicProxyAnnotation {
			raw = append(raw, i.Marshal(ctx)...)
		}
		if j.CodeBase != "" && j.descTimes == 0 {
			raw = append(raw, NewJavaString(j.CodeBase).Marshal(ctx)...) // annotation that used for set codebase
		} else {
			raw = append(raw, TC_NULL) // annotation that used for set codebase
		}
		raw = append(raw, TC_ENDBLOCKDATA)
		if obj.SuperClass == nil {
			return raw
		} else {
			raw = append(raw, obj.SuperClass.Marshal(ctx)...)
			return raw
		}
	}

	raw := []byte{TC_CLASSDESC}
	raw = append(raw, marshalString(obj.ClassName, ctx.StringCharLength)...)
	raw = append(raw, obj.SerialVersion...)
	raw = append(raw, obj.DescFlag)
	raw = append(raw, obj.Fields.Marshal(ctx)...)

	// annotation
	for _, i := range obj.Annotations {
		raw = append(raw, i.Marshal(ctx)...)
	}
	if j.CodeBase != "" && j.descTimes == 0 {
		raw = append(raw, NewJavaString(j.CodeBase).Marshal(ctx)...) // annotation that used for set codebase
	} else {
		raw = append(raw, TC_NULL) // annotation that used for set codebase
	}
	raw = append(raw, TC_ENDBLOCKDATA)

	if obj.SuperClass == nil {
		raw = append(raw, TC_NULL)
	} else {
		raw = append(raw, obj.SuperClass.Marshal(ctx)...)
	}
	return raw
}

type MarshalContext struct {
	JavaMarshaler
	DirtyDataLength  int
	StringCharLength int
}

func NewMarshalContext() *MarshalContext {
	return &MarshalContext{
		JavaMarshaler:    &CommonMarshaler{},
		DirtyDataLength:  0,
		StringCharLength: 1,
	}
}

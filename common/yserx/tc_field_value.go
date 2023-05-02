package yserx

import (
	"math"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
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

func (v *JavaFieldValue) Marshal() []byte {
	var raw []byte
	//raw = append(raw, v.FieldType)
	if v.FieldType == JT_OBJECT || v.FieldType == JT_ARRAY {
		return append(raw, v.Object.Marshal()...)
	}
	raw = append(raw, v.Bytes...)
	return raw
}

func NewJavaFieldValue(t byte, raw []byte) *JavaFieldValue {
	v := &JavaFieldValue{
		FieldType:        t,
		FieldTypeVerbose: jtToVerbose(t),
		Bytes:            raw,
	}
	initTCType(v)
	return v
}

func NewJavaFieldByteValue(b byte) *JavaFieldValue {
	return NewJavaFieldValue(JT_BYTE, []byte{b})
}

func NewJavaFieldBoolValue(b bool) *JavaFieldValue {
	var raw []byte
	if b {
		raw = append(raw, 1)
	} else {
		raw = append(raw, 0)
	}
	return NewJavaFieldValue(JT_BOOL, raw)
}

func NewJavaFieldShortValue(i int) *JavaFieldValue {
	return NewJavaFieldValue(JT_SHORT, IntTo2Bytes(i))
}

func NewJavaFieldCharValue(i int) *JavaFieldValue {
	return NewJavaFieldValue(JT_CHAR, IntTo2Bytes(i))
}

func NewJavaFieldIntValue(i uint64) *JavaFieldValue {
	return NewJavaFieldValue(JT_INT, Uint64To4Bytes(i))
}

func NewJavaFieldLongValue(i uint64) *JavaFieldValue {
	return NewJavaFieldValue(JT_LONG, Uint64To8Bytes(i))
}

func NewJavaFieldFloatValue(i float32) *JavaFieldValue {
	return NewJavaFieldValue(JT_FLOAT, Uint64To4Bytes(uint64(math.Float32bits(i))))
}

func NewJavaFieldDoubleValue(i float64) *JavaFieldValue {
	return NewJavaFieldValue(JT_DOUBLE, Uint64To8Bytes(uint64(math.Float64bits(i))))
}

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

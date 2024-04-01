package yserx

type JavaSerializable interface {
	//String() string
	//SDumper(indent int) string
	Marshal(*MarshalContext) []byte
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

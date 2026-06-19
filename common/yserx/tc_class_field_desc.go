package yserx

type JavaClassFields struct {
	Type        byte              `json:"type"`
	TypeVerbose string            `json:"type_verbose"`
	FieldCount  int               `json:"field_count"`
	Fields      []*JavaClassField `json:"fields"`
}

type JavaClassField struct {
	Type             byte             `json:"type"`
	TypeVerbose      string           `json:"type_verbose"`
	Name             string           `json:"name"`
	FieldType        byte             `json:"field_type"`
	FieldTypeVerbose string           `json:"field_type_verbose"`
	ClassName1       JavaSerializable `json:"class_name_1"`
}

func (f *JavaClassField) Marshal(cfg *MarshalContext) []byte {
	raw := []byte{f.FieldType}
	raw = append(raw, marshalString(f.Name, cfg.StringCharLength)...)

	if f.FieldType == JT_ARRAY || f.FieldType == JT_OBJECT {
		raw = append(raw, f.ClassName1.Marshal(cfg)...)
	}
	return raw
}

func (fs *JavaClassFields) Marshal(cfg *MarshalContext) []byte {
	raw := IntTo2Bytes(fs.FieldCount)
	for _, i := range fs.Fields {
		raw = append(raw, i.Marshal(cfg)...)
	}
	return raw
}

// NewJavaClassFields 创建一个 Java 类字段描述集合，用于聚合多个字段描述
// 在 yak 中通过 java.NewJavaClassFields 调用
// 参数:
//   - fields: 零个或多个字段描述对象
//
// 返回值:
//   - Java 类字段描述集合对象
//
// Example:
// ```
// // 该示例为示意性用法：构造字段描述集合
// f = java.NewJavaClassField("id", 0x49, nil)
// fields = java.NewJavaClassFields(f)
// println(len(fields.Fields))
// ```
func NewJavaClassFields(fields ...*JavaClassField) *JavaClassFields {
	f := &JavaClassFields{
		FieldCount: len(fields),
		Fields:     fields,
	}
	initTCType(f)
	return f
}

// NewJavaClassField 创建一个 Java 类字段描述，描述单个字段的名称与类型
// 在 yak 中通过 java.NewJavaClassField 调用
// 参数:
//   - name: 字段名
//   - jType: 字段类型标记(如 0x49 表示 int、0x4c 表示对象)
//   - className: 对象类型字段的类名描述对象，基本类型可传 nil
//
// 返回值:
//   - Java 类字段描述对象
//
// Example:
// ```
// // 该示例为示意性用法：构造一个 int 字段描述
// f = java.NewJavaClassField("id", 0x49, nil)
// println(f.Name)
// ```
func NewJavaClassField(name string, jType byte, className JavaSerializable) *JavaClassField {
	f := &JavaClassField{
		Name:             name,
		FieldType:        jType,
		FieldTypeVerbose: jtToVerbose(jType),
		ClassName1:       className,
	}
	initTCType(f)
	return f
}

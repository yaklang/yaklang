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

func NewJavaClassFields(fields ...*JavaClassField) *JavaClassFields {
	f := &JavaClassFields{
		FieldCount: len(fields),
		Fields:     fields,
	}
	initTCType(f)
	return f
}

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

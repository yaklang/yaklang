package yserx

type JavaEnumDesc struct {
	Type          byte             `json:"type"`
	TypeVerbose   string           `json:"type_verbose"`
	TypeClassDesc JavaSerializable `json:"type_class_desc"`
	ConstantName  JavaSerializable `json:"constant_name"`
	Handle        uint64           `json:"handle"`
}

func (desc *JavaEnumDesc) Marshal() []byte {
	raw := []byte{TC_ENUM}
	raw = append(raw, desc.TypeClassDesc.Marshal()...)
	raw = append(raw, desc.ConstantName.Marshal()...)
	return raw
}

func NewJavaEnum(i *JavaClassDesc, constantName *JavaString) *JavaEnumDesc {
	d := &JavaEnumDesc{
		Type:          TC_ENUM,
		TypeVerbose:   tcToVerbose(TC_ENUM),
		TypeClassDesc: i,
		ConstantName:  constantName,
	}
	initTCType(d)
	return d
}

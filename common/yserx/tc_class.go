package yserx

type JavaClass struct {
	Type        byte             `json:"type"`
	TypeVerbose string           `json:"type_verbose"`
	Desc        JavaSerializable `json:"class_desc"`
	Handle      uint64           `json:"handle"`
}

func (j *JavaClass) Marshal() []byte {
	raw := []byte{TC_CLASS}
	raw = append(raw, j.Desc.Marshal()...)
	return raw
}

func NewJavaClass(j *JavaClassDesc) *JavaClass {
	c := &JavaClass{Desc: j, TypeVerbose: tcToVerbose(TC_CLASS), Type: TC_CLASS}
	initTCType(c)
	return c
}

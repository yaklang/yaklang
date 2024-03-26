package yserx

type JavaObject struct {
	Type        byte               `json:"type"`
	TypeVerbose string             `json:"type_verbose"`
	Class       JavaSerializable   `json:"class_desc"`
	ClassData   []JavaSerializable `json:"class_data"`
	Handle      uint64             `json:"handle"`
}

const INDENT = "  "

func (j *JavaObject) Marshal(cfg *MarshalContext) []byte {
	var raw = []byte{TC_OBJECT}

	raw = append(raw, j.Class.Marshal(cfg)...)
	for _, i := range j.ClassData {
		raw = append(raw, i.Marshal(cfg)...)
	}
	return raw
}

func NewJavaObject(class *JavaClassDesc, classData ...*JavaClassData) *JavaObject {
	obj := &JavaObject{
		TypeVerbose: tcToVerbose(TC_OBJECT),
		Type:        TC_OBJECT,
	}

	obj.Class = class
	var rest []JavaSerializable
	for _, i := range classData {
		rest = append(rest, i)
	}
	obj.ClassData = rest
	initTCType(obj)
	return obj
}
func (j *JavaObject) Bytes() []byte {
	return MarshalJavaObjects(j)
}
func (j *JavaObject) Json() (string, error) {
	jd, err := ToJson(j)
	return string(jd), err
}

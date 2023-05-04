package yserx

type JavaClassData struct {
	Type        byte               `json:"type"`
	TypeVerbose string             `json:"type_verbose"`
	Fields      []JavaSerializable `json:"fields"`
	BlockData   []JavaSerializable `json:"block_data"`
}

func NewJavaClassData(fields []JavaSerializable, blockData []JavaSerializable) *JavaClassData {
	c := &JavaClassData{}
	initTCType(c)
	c.Fields = fields
	c.BlockData = blockData
	return c
}

func (d *JavaClassData) Marshal() []byte {
	var raw []byte
	for _, f := range d.Fields {
		raw = append(raw, f.Marshal()...)
	}
	for _, b := range d.BlockData {
		raw = append(raw, b.Marshal()...)
	}
	return raw
}

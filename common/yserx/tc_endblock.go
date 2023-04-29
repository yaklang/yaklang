package yserx

type JavaEndBlockData struct {
	Type        byte   `json:"type"`
	TypeVerbose string `json:"type_verbose"`
	IsEmpty     bool   `json:"is_empty,omitempty"`
}

func (j *JavaEndBlockData) Marshal() []byte {
	if j.IsEmpty {
		return nil
	}
	return []byte{TC_ENDBLOCKDATA}
}

func NewJavaEndBlockData() *JavaEndBlockData {
	return &JavaEndBlockData{}
}

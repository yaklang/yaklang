package yserx

type JavaNull struct {
	Type        byte   `json:"type"`
	TypeVerbose string `json:"type_verbose"`
	IsEmpty     bool   `json:"is_empty,omitempty"`
}

func (s *JavaNull) Marshal() []byte {
	if s.IsEmpty {
		return nil
	}
	return []byte{TC_NULL}
}

func NewJavaNull() *JavaNull {
	return &JavaNull{
		TypeVerbose: tcToVerbose(TC_NULL),
		Type:        TC_NULL,
	}
}

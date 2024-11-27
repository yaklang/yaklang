package yserx

import "github.com/yaklang/yaklang/common/utils"

type JavaString struct {
	Type        byte   `json:"type"`
	TypeVerbose string `json:"type_verbose"`
	IsLong      bool   `json:"is_long"`
	Size        uint64 `json:"size"`
	Raw         []byte `json:"raw"`
	Value       string `json:"value"`
	Handle      uint64 `json:"handle"`
}

func (s *JavaString) Marshal(cfg *MarshalContext) []byte {
	if s.IsLong {
		raw := []byte{TC_LONGSTRING}
		raw = append(raw, Uint64To8Bytes(s.Size)...)
		raw = append(raw, s.Raw...)
		return raw
	}
	raw := []byte{TC_STRING}
	byts := utils.Utf8EncodeBySpecificLength(s.Raw, cfg.StringCharLength)
	raw = append(raw, IntTo2Bytes(len(byts))...)
	raw = append(raw, byts...)
	return raw
}

func NewJavaString(raw string) *JavaString {
	return &JavaString{
		Type:        TC_STRING,
		TypeVerbose: tcToVerbose(TC_STRING),
		IsLong:      false,
		Size:        uint64(len(raw)),
		Raw:         []byte(raw),
		Value:       raw,
	}
}

func NewJavaLongString(raw string) *JavaString {
	s := &JavaString{
		Type:        TC_LONGSTRING,
		TypeVerbose: tcToVerbose(TC_LONGSTRING),
		IsLong:      true,
		Size:        uint64(len(raw)),
		Raw:         []byte(raw),
		Value:       raw,
	}
	initTCType(s)
	return s
}

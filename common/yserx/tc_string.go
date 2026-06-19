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

// NewJavaString 创建一个 Java 序列化的普通字符串对象(TC_STRING)
// 在 yak 中通过 java.NewJavaString 调用
// 参数:
//   - raw: 字符串内容
//
// 返回值:
//   - Java 字符串序列化对象
//
// Example:
// ```
// s = java.NewJavaString("hello")
// println(s.Value) // OUT: hello
// ```
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

// NewJavaLongString 创建一个 Java 序列化的长字符串对象(TC_LONGSTRING)，用于超长字符串
// 在 yak 中通过 java.NewJavaLongString 调用
// 参数:
//   - raw: 字符串内容
//
// 返回值:
//   - Java 长字符串序列化对象
//
// Example:
// ```
// s = java.NewJavaLongString("hello")
// println(s.Value) // OUT: hello
// ```
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

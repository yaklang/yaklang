package yserx

type JavaNull struct {
	Type        byte   `json:"type"`
	TypeVerbose string `json:"type_verbose"`
	IsEmpty     bool   `json:"is_empty,omitempty"`
}

func (s *JavaNull) Marshal(cfg *MarshalContext) []byte {
	if s.IsEmpty {
		return nil
	}
	return []byte{TC_NULL}
}

// NewJavaNull 创建一个 Java 序列化的 null 对象(TC_NULL)
// 在 yak 中通过 java.NewJavaNull 调用，常用于表示空引用字段
// 返回值:
//   - Java null 序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造 null 对象并参与序列化
// n = java.NewJavaNull()
// b = java.MarshalJavaObjects(n)
// println(len(b))
// ```
func NewJavaNull() *JavaNull {
	return &JavaNull{
		TypeVerbose: tcToVerbose(TC_NULL),
		Type:        TC_NULL,
	}
}

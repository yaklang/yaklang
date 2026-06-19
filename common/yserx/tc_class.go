package yserx

type JavaClass struct {
	Type        byte             `json:"type"`
	TypeVerbose string           `json:"type_verbose"`
	Desc        JavaSerializable `json:"class_desc"`
	Handle      uint64           `json:"handle"`
}

func (j *JavaClass) Marshal(cfg *MarshalContext) []byte {
	raw := []byte{TC_CLASS}
	raw = append(raw, j.Desc.Marshal(cfg)...)
	return raw
}

// NewJavaClass 创建一个 Java 类对象(TC_CLASS)，用于序列化对 Class 本身的引用
// 在 yak 中通过 java.NewJavaClass 调用
// 参数:
//   - j: 类描述对象
//
// 返回值:
//   - Java 类序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造类对象
// desc = java.NewJavaClassDesc("com.example.Foo", []byte{0,0,0,0,0,0,0,1}, 0x02, java.NewJavaClassFields(), nil, nil)
// cls = java.NewJavaClass(desc)
// println(cls.TypeVerbose)
// ```
func NewJavaClass(j *JavaClassDesc) *JavaClass {
	c := &JavaClass{Desc: j, TypeVerbose: tcToVerbose(TC_CLASS), Type: TC_CLASS}
	initTCType(c)
	return c
}

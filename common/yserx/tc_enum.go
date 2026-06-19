package yserx

type JavaEnumDesc struct {
	Type          byte             `json:"type"`
	TypeVerbose   string           `json:"type_verbose"`
	TypeClassDesc JavaSerializable `json:"type_class_desc"`
	ConstantName  JavaSerializable `json:"constant_name"`
	Handle        uint64           `json:"handle"`
}

func (desc *JavaEnumDesc) Marshal(cfg *MarshalContext) []byte {
	raw := []byte{TC_ENUM}
	raw = append(raw, desc.TypeClassDesc.Marshal(cfg)...)
	raw = append(raw, desc.ConstantName.Marshal(cfg)...)
	return raw
}

// NewJavaEnum 创建一个 Java 枚举对象(TC_ENUM)，由枚举类描述与常量名组成
// 在 yak 中通过 java.NewJavaEnum 调用
// 参数:
//   - i: 枚举类型的类描述对象
//   - constantName: 枚举常量名(Java 字符串对象)
//
// 返回值:
//   - Java 枚举序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造枚举对象
// desc = java.NewJavaClassDesc("java.lang.Enum", []byte{0,0,0,0,0,0,0,0}, 0x02, java.NewJavaClassFields(), nil, nil)
// e = java.NewJavaEnum(desc, java.NewJavaString("RED"))
// println(e.TypeVerbose)
// ```
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

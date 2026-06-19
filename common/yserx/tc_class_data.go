package yserx

type JavaClassData struct {
	Type        byte               `json:"type"`
	TypeVerbose string             `json:"type_verbose"`
	Fields      []JavaSerializable `json:"fields"`
	BlockData   []JavaSerializable `json:"block_data"`
}

// NewJavaClassData 创建一个 Java 类数据对象，承载对象实例的字段值与自定义块数据
// 在 yak 中通过 java.NewJavaClassData 调用，配合 java.NewJavaObject 使用
// 参数:
//   - fields: 字段值序列化对象列表
//   - blockData: 自定义块数据序列化对象列表
//
// 返回值:
//   - Java 类数据对象
//
// Example:
// ```
// // 该示例为示意性用法：构造类数据
// data = java.NewJavaClassData([java.NewJavaFieldIntValue(1)], [])
// println(len(data.Fields))
// ```
func NewJavaClassData(fields []JavaSerializable, blockData []JavaSerializable) *JavaClassData {
	c := &JavaClassData{}
	initTCType(c)
	c.Fields = fields
	c.BlockData = blockData
	return c
}

func (d *JavaClassData) Marshal(cfg *MarshalContext) []byte {
	var raw []byte
	for _, f := range d.Fields {
		raw = append(raw, f.Marshal(cfg)...)
	}
	for _, b := range d.BlockData {
		raw = append(raw, b.Marshal(cfg)...)
	}
	return raw
}

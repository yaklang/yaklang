package yserx

type JavaEndBlockData struct {
	Type        byte   `json:"type"`
	TypeVerbose string `json:"type_verbose"`
	IsEmpty     bool   `json:"is_empty,omitempty"`
}

func (j *JavaEndBlockData) Marshal(cfg *MarshalContext) []byte {
	if j.IsEmpty {
		return nil
	}
	return []byte{TC_ENDBLOCKDATA}
}

// NewJavaEndBlockData 创建一个 Java 序列化的块数据结束标记对象(TC_ENDBLOCKDATA)
// 在 yak 中通过 java.NewJavaEndBlockData 调用，用于标记自定义块数据写入结束
// 返回值:
//   - Java 块数据结束标记序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造块数据结束标记
// end = java.NewJavaEndBlockData()
// println(end.TypeVerbose)
// ```
func NewJavaEndBlockData() *JavaEndBlockData {
	return &JavaEndBlockData{}
}

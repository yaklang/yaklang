package yserx

import (
	"bufio"
	"bytes"
)

type JavaReference struct {
	Type        byte   `json:"type"`
	TypeVerbose string `json:"type_verbose"`
	Value       []byte `json:"value"`
	Handle      uint64 `json:"handle"`
}

func (j *JavaReference) GetHandle() uint64 {
	r, err := Read4ByteToUint64(bufio.NewReader(bytes.NewBuffer(j.Value)))
	if err != nil {
		return 0
	}
	return r
}

func (j *JavaReference) Marshal(cfg *MarshalContext) []byte {
	return append([]byte{TC_REFERENCE}, j.Value...)
}

// NewJavaReference 创建一个 Java 序列化的引用对象(TC_REFERENCE)，通过句柄复用已序列化对象
// 在 yak 中通过 java.NewJavaReference 调用
// 参数:
//   - handle: 被引用对象的句柄(从 0x7e0000 起递增)
//
// 返回值:
//   - Java 引用序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造对已有对象的引用
// ref = java.NewJavaReference(0x7e0000)
// println(ref.TypeVerbose)
// ```
func NewJavaReference(handle uint64) *JavaReference {
	r := &JavaReference{
		Type:        TC_REFERENCE,
		TypeVerbose: tcToVerbose(TC_REFERENCE),
		Value:       Uint64To4Bytes(handle),
		Handle:      handle,
	}
	initTCType(r)
	return r
}

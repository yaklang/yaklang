package yserx

import (
	"bytes"
	"github.com/yaklang/yaklang/common/go-funk"
)

type JavaArray struct {
	Type        byte              `json:"type"`
	TypeVerbose string            `json:"type_verbose"`
	ClassDesc   JavaSerializable  `json:"class_desc"`
	Size        int               `json:"size"`
	Values      []*JavaFieldValue `json:"values"`
	Handle      uint64            `json:"handle"`
	Bytescode   bool              `json:"bytescode,omitempty"`
	Bytes       []byte            `json:"bytes,omitempty"`
}

func (j *JavaArray) Marshal(cfg *MarshalContext) []byte {
	var rawBuffer bytes.Buffer
	rawBuffer.WriteByte(TC_ARRAY)
	rawBuffer.Write(j.ClassDesc.Marshal(cfg))
	rawBuffer.Write(IntTo4Bytes(j.Size))
	if j.Bytescode {
		funk.ForEach(j.Bytes, func(i byte) {
			rawBuffer.Write(NewJavaFieldByteValue(i).Marshal(cfg))
		})
		return rawBuffer.Bytes()
	}
	funk.ForEach(j.Values, func(i JavaSerializable) {
		rawBuffer.Write(i.Marshal(cfg))
	})
	return rawBuffer.Bytes()
}

func (ret *JavaArray) fixBytescode() {
	if ret.Bytescode {
		return
	}
	// 初始化成功之后，看一下 是不是 Bytescode
	if ret.ClassDesc != nil {
		switch classDesc := ret.ClassDesc.(type) {
		case *JavaReference:
			if len(ret.Values) > 100 {
				ret.Bytescode = true
				for _, value := range ret.Values[:100] {
					if value.FieldType != JT_BYTE {
						ret.Bytescode = false
						break
					}
				}
			}
		case *JavaClassDesc:
			if classDesc.Detail != nil {
				ret.Bytescode = classDesc.Detail.ClassName == "[B"
				break
			}
		}
	}

	if ret.Bytescode {
		ret.Bytes = make([]byte, ret.Size)
		for i, value := range ret.Values {
			if len(value.Bytes) > 0 {
				ret.Bytes[i] = value.Bytes[0]
			}
		}
		ret.Values = nil
	}
}

// NewJavaArray 创建一个 Java 数组对象(TC_ARRAY)，承载同类型元素的字段值序列
// 在 yak 中通过 java.NewJavaArray 调用
// 参数:
//   - j: 数组的类描述对象(描述元素类型)
//   - values: 零个或多个数组元素字段值
//
// 返回值:
//   - Java 数组序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造一个 int 数组对象
// desc = java.NewJavaClassDesc("[I", []byte{0,0,0,0,0,0,0,1}, 0x02, java.NewJavaClassFields(), nil, nil)
// arr = java.NewJavaArray(desc, java.NewJavaFieldIntValue(1), java.NewJavaFieldIntValue(2))
// println(arr.TypeVerbose)
// ```
func NewJavaArray(j *JavaClassDesc, values ...*JavaFieldValue) *JavaArray {
	a := &JavaArray{
		ClassDesc: j,
		Size:      len(values),
		Values:    values,
	}
	initTCType(a)
	a.fixBytescode()
	return a
}

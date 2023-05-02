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

func (j *JavaArray) Marshal() []byte {
	var rawBuffer bytes.Buffer
	rawBuffer.WriteByte(TC_ARRAY)
	rawBuffer.Write(j.ClassDesc.Marshal())
	rawBuffer.Write(IntTo4Bytes(j.Size))
	if j.Bytescode {
		funk.ForEach(j.Bytes, func(i byte) {
			rawBuffer.Write(NewJavaFieldByteValue(i).Marshal())
		})
		return rawBuffer.Bytes()
	}
	funk.ForEach(j.Values, func(i JavaSerializable) {
		rawBuffer.Write(i.Marshal())
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

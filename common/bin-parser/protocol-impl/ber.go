package protocol_impl

import (
	"encoding/json"
	"errors"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/utils"
	"io"
)

type BERElementType struct {
	Class       byte // 占前两位
	Constructed bool // 第三位
	TagNumber   byte // 后五位
}

const (
	BERUniversalData       = 0
	BERApplicationData     = 1
	BERContextSpecificData = 2
	BERPrivateData         = 3

	BERInteger = 0x02
	BERString  = 0x1b
)

type BERElement struct {
	Type  *BERElementType
	Value any
}

func (b *BERElement) ToMap() map[string]any {
	return nil
}
func LoadFromMap(d map[string]any) *BERElement {
	return nil
}
func (b *BERElement) Marshal() ([]byte, error) {
	d, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	dataMap := map[string]any{}
	err = json.Unmarshal(d, &dataMap)
	if err != nil {
		return nil, err
	}
	res, err := parser.GenerateBinary(dataMap, "application-layer.ber", "BER")
	if err != nil {
		return nil, err
	}
	return utils.NodeToBytes(res), nil
}
func NewBER(data any) (*BERElement, error) {
	switch data.(type) {
	case string:
		return &BERElement{
			Type: &BERElementType{
				Class:       BERUniversalData,
				Constructed: false,
				TagNumber:   BERString,
			},
			Value: data,
		}, nil
	case int:
		return &BERElement{
			Type: &BERElementType{
				Class:       BERUniversalData,
				Constructed: false,
				TagNumber:   BERInteger,
			},
			Value: data,
		}, nil
	case []*BERElement:
		return &BERElement{
			Type: &BERElementType{
				Class:       BERUniversalData,
				Constructed: true,
				TagNumber:   0,
			},
			Value: data,
		}, nil
	default:
		return nil, errors.New("not support data type")
	}
}
func ParseBER(r io.Reader) (*BERElement, error) {
	res, err := parser.ParseBinary(r, "application-layer.ber", "BER")
	if err != nil {
		return nil, err
	}
	data := utils.NodeToData(res)
	d, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	berIns := &BERElement{}
	err = json.Unmarshal(d, berIns)
	if err != nil {
		return nil, err
	}
	return berIns, nil
}

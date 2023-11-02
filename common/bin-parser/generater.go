package bin_parser

import (
	"errors"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"os"
)

func Generate(data map[string]any, rule string) ([]byte, error) {
	ruleContent, err := os.ReadFile("./rules/" + rule + ".yaml")
	if err != nil {
		return nil, err
	}
	var ruleMap yaml.MapSlice
	err = yaml.Unmarshal(ruleContent, &ruleMap)
	if err != nil {
		return nil, err
	}
	opts, ruleMap1 := splitConfigAndNode(ruleMap)
	res, err := generate(data, ruleMap1, opts)
	if err != nil {
		return nil, err
	}
	return res.GetBytes(), nil
}
func getValueFromMap(data any, k string) any {
	mapData, err := utils.InterfaceToMapInterfaceE(data)
	if err != nil {
		return nil
	}
	return mapData[k]
}
func generate(data any, rule any, opts []ConfigFunc) (binx.ResultIf, error) {
	switch ret := rule.(type) {
	case yaml.MapSlice:
		listRes := binx.NewListResult()
		listRes.Length = len(ret)
		for _, v := range ret {
			opts1, v1 := splitConfigAndNode(v)
			r, err := generate(data, v1, append(opts, opts1...))
			if err != nil {
				return nil, err
			}
			listRes.Result = append(listRes.Result, r)
			listRes.Bytes = append(listRes.Bytes, r.GetBytes()...)
		}
		return listRes, nil
	case yaml.MapItem:
		switch ret.Value.(type) {
		case string:
			val := getValueFromMap(data, utils.InterfaceToString(ret.Key))
			if val == nil {
				return nil, utils.Errorf("key `%s` not found in data", ret.Key)
			}
			config := NewConfig(opts)
			var newRes func(any) (binx.ResultIf, error)
			newRes = func(val any) (binx.ResultIf, error) {
				res := binx.NewResult([]byte{})
				switch config.endian {
				case "big":
					res.ByteOrder = binx.BigEndianByteOrder
				case "little":
					res.ByteOrder = binx.LittleEndianByteOrder
				default:
					res.ByteOrder = binx.BigEndianByteOrder
				}

				switch ret := val.(type) {
				case uint8:
					res.Type = binx.Uint8
					res.ResultBase.Bytes = []byte{ret}
					return res, nil
				case uint16:
					res.Type = binx.Uint16
					raw := make([]byte, 2)
					raw[0] = byte(ret >> 8)
					raw[1] = byte(ret)
					res.ResultBase.Bytes = raw
					return res, nil
				case uint32:
					res.Type = binx.Uint32
					raw := make([]byte, 4)
					raw[0] = byte(ret >> 24)
					raw[1] = byte(ret >> 16)
					raw[2] = byte(ret >> 8)
					raw[3] = byte(ret)
					res.ResultBase.Bytes = raw
					return res, nil
				case uint64:
					res.Type = binx.Uint64
					raw := make([]byte, 8)
					raw[0] = byte(ret >> 56)
					raw[1] = byte(ret >> 48)
					raw[2] = byte(ret >> 40)
					raw[3] = byte(ret >> 32)
					raw[4] = byte(ret >> 24)
					raw[5] = byte(ret >> 16)
					raw[6] = byte(ret >> 8)
					raw[7] = byte(ret)
					res.ResultBase.Bytes = raw
					return res, nil
				case int8:
					return newRes(uint8(ret))
				case int16:
					return newRes(uint16(ret))
				case int32:
					return newRes(uint32(ret))
				case int64:
					return newRes(uint64(ret))
				case string:
					res.Type = binx.Bytes
					res.ResultBase.Bytes = []byte(ret)
					return res, nil
				case []byte:
					res.Type = binx.Bytes
					res.ResultBase.Bytes = ret
					return res, nil
				case bool:
					res.Type = binx.Bool
					if ret {
						res.ResultBase.Bytes = []byte{1}
					} else {
						res.ResultBase.Bytes = []byte{0}
					}
					return res, nil
				case yaml.MapSlice:
					return generate(data, ret, opts)
				default:
					return nil, errors.New("unknown type")
				}
			}
			return newRes(val)
		case yaml.MapSlice:
			val := getValueFromMap(data, utils.InterfaceToString(ret.Key))
			if val == nil {
				return nil, utils.Errorf("key `%s` not found in data", ret.Key)
			}
			return generate(val, ret.Value, opts)
		default:
			return nil, errors.New("unknown type")
		}
	default:
		return nil, errors.New("unknown type")
	}
}

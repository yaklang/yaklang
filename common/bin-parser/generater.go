package bin_parser

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"os"
	"reflect"
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
	opts1, node := splitConfigAndNode(rule)
	opts = append(opts, opts1...)
	switch ret := node.(type) {
	case yaml.MapSlice:
		listRes := binx.NewListResult()
		listRes.Length = len(ret)
		for _, v := range ret {
			r, err := generate(data, v, opts)
			if err != nil {
				return nil, err
			}
			listRes.Result = append(listRes.Result, r)
			listRes.Bytes = append(listRes.Bytes, r.GetBytes()...)
		}
		return listRes, nil
	case yaml.MapItem:
		opts1, node := splitConfigAndNode(ret.Value)
		opts := append(opts, opts1...)
		ret.Value = node
		switch ret.Value.(type) {
		case string:
			val := getValueFromMap(data, utils.InterfaceToString(ret.Key))
			if val == nil {
				return nil, utils.Errorf("key `%s` not found in data", ret.Key)
			}
			var newRes func(any) (binx.ResultIf, error)
			newRes = func(val any) (binx.ResultIf, error) {
				opts1, err := parseTerminalNode(ret.Value.(string))
				if err != nil {
					return nil, fmt.Errorf("parse terminal node error: %w", err)
				}
				config := NewConfig(append(opts, opts1...))
				res := binx.NewResult([]byte{})
				res.ByteOrder = config.endian
				switch config.dataType {
				case binx.Int8, binx.Uint8, binx.Int16, binx.Uint16, binx.Int32, binx.Uint32, binx.Int64, binx.Uint64:
					var number uint64 = 0
					switch reflect.TypeOf(val).Kind() {
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						number = uint64(reflect.ValueOf(val).Int())
					case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
						number = reflect.ValueOf(val).Uint()
					default:
						return nil, fmt.Errorf("expect integer type, but got %v", reflect.TypeOf(val).Kind())
					}
					bytes := make([]byte, 8)
					if res.ByteOrder == binx.LittleEndianByteOrder {
						binary.LittleEndian.PutUint64(bytes, number)
						if config.length > 8 {
							bytes = append(bytes, make([]byte, config.length-8)...)
						} else {
							bytes = bytes[:config.length]
						}
					} else {
						binary.BigEndian.PutUint64(bytes, number)
						if config.length > 8 {
							bytes = append(make([]byte, config.length-8), bytes...)
						} else {
							bytes = bytes[8-config.length:]
						}
					}
					res.Bytes = bytes
					res.Identifier = utils.InterfaceToString(ret.Key)
					return res, nil
				default:
					res.Bytes = utils.InterfaceToBytes(val)
					res.Identifier = utils.InterfaceToString(ret.Key)
					return res, nil
				}
			}
			return newRes(val)
		default:
			val := getValueFromMap(data, utils.InterfaceToString(ret.Key))
			if val == nil {
				return nil, utils.Errorf("key `%s` not found in data", ret.Key)
			}
			return generate(val, ret.Value, opts)
		}
	default:
		return nil, errors.New("unknown type")
	}
}

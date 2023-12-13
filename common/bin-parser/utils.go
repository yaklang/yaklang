package bin_parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"gopkg.in/yaml.v2"
	"reflect"
)

func NodeToMap(node *base.Node) any {
	if node.Cfg.Has(stream_parser.CfgNodeResult) {
		return stream_parser.GetResultByNode(node)
	}
	if node.Cfg.GetBool(stream_parser.CfgIsList) {
		res := []any{}
		for _, sub := range node.Children {
			d := NodeToMap(sub)
			if d != nil {
				res = append(res, d)
			}
		}
		if len(res) == 0 {
			return nil
		}
		return res
	} else {
		res := map[string]any{}
		for _, sub := range node.Children {
			d := NodeToMap(sub)
			if d != nil {
				res[sub.Name] = NodeToMap(sub)
			}
		}
		if len(res) == 0 {
			return nil
		}
		return res
	}
}
func NodeToBytes(node *base.Node) []byte {
	buffer := node.Ctx.GetItem("buffer").(*bytes.Buffer)
	return buffer.Bytes()
	//res := []byte{}
	//var toBytes func(nodeRes *base.Node)
	//toBytes = func(node *base.Node) {
	//	if stream_parser.NodeHasResult(node) {
	//		res = append(res, stream_parser.GetBytesByNode(node)...)
	//	} else {
	//		for _, sub := range node.Children {
	//			toBytes(sub)
	//		}
	//	}
	//}
	//toBytes(node)
	//return res
}
func DumpNode(node *base.Node) {
	println(nodeResultToYaml(node))
}
func SdumpNode(node *base.Node) string {
	return nodeResultToYaml(node)
}

func nodeResultToYaml(node *base.Node) (result string) {
	var toMap func(nodeRes *base.Node) any
	_ = toMap

	toMap = func(node *base.Node) any {
		if stream_parser.NodeHasResult(node) {
			data := stream_parser.GetResultByNode(node)
			if v, ok := data.([]byte); ok {
				data = fmt.Sprintf("%x", v)
			}
			return data
		} else {
			res := yaml.MapSlice{}
			for _, sub := range node.Children {
				res = append(res, yaml.MapItem{
					Key:   sub.Name,
					Value: toMap(sub),
				})
			}
			return res
		}
	}
	//nodeRes := node.Cfg.GetItem(stream_parser.CfgNodeResult).(*stream_parser.NodeResult)
	res, err := yaml.Marshal(toMap(node))
	if err != nil {
		log.Errorf("error when marshal node to yaml: %v", err)
	}
	return string(res)
}
func ToUint64(d any) (uint64, error) {
	switch ret := d.(type) {
	case uint64:
		return ret, nil
	case uint32:
		return uint64(ret), nil
	case uint16:
		return uint64(ret), nil
	case uint8:
		return uint64(ret), nil
	case int64:
		return uint64(ret), nil
	case int32:
		return uint64(ret), nil
	case int16:
		return uint64(ret), nil
	case int8:
		return uint64(ret), nil
	case int:
		return uint64(ret), nil
	default:
		return 0, fmt.Errorf("unexpected type: %v", reflect.TypeOf(d))
	}
}
func ResultToJson(d any) (string, error) {
	var toRawData func(d any) any
	toRawData = func(d any) any {
		refV := reflect.ValueOf(d)
		switch ret := d.(type) {
		case []uint8:
			return string(ret)
		}
		if !refV.CanAddr() {
			return d
		}
		if refV.Kind() == reflect.Slice || refV.Kind() == reflect.Array {
			for i := 0; i < refV.Len(); i++ {
				refV.Index(i).Set(reflect.ValueOf(toRawData(refV.Index(i).Interface())))
			}
			return refV.Interface()
		} else if refV.Kind() == reflect.Map {
			for _, k := range refV.MapKeys() {
				refV.SetMapIndex(k, reflect.ValueOf(toRawData(refV.MapIndex(k).Interface())))
			}
			return refV.Interface()
		} else {
			return d
		}
	}
	rawData := toRawData(d)
	res, err := json.Marshal(rawData)
	if err != nil {
		return "", err
	}
	return string(res), nil
}
func JsonToResult(jsonStr string) (any, error) {
	d := map[string]any{}
	err := json.Unmarshal([]byte(jsonStr), &d)
	if err != nil {
		return nil, err
	}
	var toRawDataErr error
	var toRawData func(d any) any
	toRawData = func(d any) any {
		refV := reflect.ValueOf(d)
		if refV.Kind() == reflect.Slice || refV.Kind() == reflect.Array {
			for i := 0; i < refV.Len(); i++ {
				refV.Index(i).Set(reflect.ValueOf(toRawData(refV.Index(i).Interface())))
			}
			return refV.Interface()
		} else if refV.Kind() == reflect.Map {
			if len(refV.MapKeys()) == 1 {
				refKey := refV.MapKeys()[0]
				if v, ok := refKey.Interface().(string); ok && v == "__data__" {
					if v, ok = refV.MapIndex(refKey).Interface().(string); ok {
						res, err := codec.DecodeBase64(v)
						if err != nil {
							toRawDataErr = err
						}
						return res
					}
				}
			}
			for _, k := range refV.MapKeys() {
				refV.SetMapIndex(k, reflect.ValueOf(toRawData(refV.MapIndex(k).Interface())))
			}
			return refV.Interface()
		} else {
			return d
		}
	}
	rawData := toRawData(d)
	if toRawDataErr != nil {
		return nil, toRawDataErr
	}
	return rawData, nil
}

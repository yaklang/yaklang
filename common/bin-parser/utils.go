package bin_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/log"
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

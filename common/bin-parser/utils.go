package bin_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/log"
	"gopkg.in/yaml.v2"
	"reflect"
)

func DumpNode(node *base.Node) {
	println(nodeResultToYaml(node))
}
func SdumpNode(node *base.Node) string {
	return nodeResultToYaml(node)
}

func nodeResultToYaml(node *base.Node) (result string) {
	var toMap func(nodeRes *stream_parser.NodeResult) any
	_ = toMap
	toMap = func(nodeRes *stream_parser.NodeResult) any {
		if node.Name == "Option" {
			print()
		}
		if len(nodeRes.Sub) == 0 {
			data, err := nodeRes.Result()
			if err != nil {
				log.Errorf("error when get node result: %v", err)
				return nil
			}
			if v, ok := data.([]byte); ok {
				data = fmt.Sprintf("%x", v)
			}
			return data
		} else {
			res := yaml.MapSlice{}
			for _, subRes := range nodeRes.Sub {
				res = append(res, yaml.MapItem{
					Key:   subRes.Node.Name,
					Value: toMap(subRes),
				})
			}
			return res
			//if !node.Cfg.GetBool("isList") {
			//	res := yaml.MapSlice{}
			//	for _, child := range node.Children {
			//		res = append(res, yaml.MapItem{
			//			Key:   child.Name,
			//			Value: toMap(child),
			//		})
			//	}
			//	return res
			//} else {
			//	res := []any{}
			//	for _, child := range node.Children {
			//		res = append(res, toMap(child))
			//	}
			//	return res
			//}
		}
	}
	//	return ""
	//}
	nodeRes := node.Cfg.GetItem(stream_parser.CfgNodeResult).(*stream_parser.NodeResult)
	res, err := yaml.Marshal(toMap(nodeRes))
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

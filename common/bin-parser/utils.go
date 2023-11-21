package bin_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"reflect"
)

func DumpNode(node *base.Node) {
	println(nodeResultToYaml(node))
}
func SdumpNode(node *base.Node) string {
	return nodeResultToYaml(node)
}

func nodeResultToYaml(node *base.Node) (result string) {
	var toMap func(node *base.Node) any
	_ = toMap
	toMap = func(node *base.Node) any {
		//if node.IsTerminalData && len(node.Children) == 0 {
		//	data := node.TerminalData
		//	if v, ok := data.([]byte); ok {
		//		data = fmt.Sprintf("%x", v)
		//	}
		//	return data
		//} else {
		//	if !node.Struct.Cfg.GetBool("isList") {
		//		res := yaml.MapSlice{}
		//		for _, child := range node.Children {
		//			res = append(res, yaml.MapItem{
		//				Key:   child.Struct.Name,
		//				Value: toMap(child),
		//			})
		//		}
		//		return res
		//	} else {
		//		res := []any{}
		//		for _, child := range node.Children {
		//			res = append(res, toMap(child))
		//		}
		//		return res
		//	}
		return nil
	}
	//	return ""
	//}
	//res1, err := yaml.Marshal(toMap(node))
	//if err != nil {
	//	log.Errorf("error when marshal node to yaml: %v", err)
	//}
	//_= res1
	//return string(res)
	return ""
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

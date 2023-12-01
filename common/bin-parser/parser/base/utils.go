package base

import (
	"reflect"
	"strconv"
	"strings"
)

func GetSubData(d any, key string) (any, bool) {
	p := strings.Split(key, ".")
	for _, ele := range p {
		refV := reflect.ValueOf(d)
		if refV.Kind() == reflect.Map {
			v := refV.MapIndex(reflect.ValueOf(ele))
			if !v.IsValid() {
				return nil, false
			} else {
				d = v.Interface()
			}
		} else if refV.Kind() == reflect.Slice || refV.Kind() == reflect.Array {
			if !strings.HasPrefix(ele, "#") {
				return nil, false
			}
			index, err := strconv.Atoi(ele[1:])
			if err != nil {
				return nil, false
			}
			if index >= refV.Len() {
				return nil, false
			}
			d = refV.Index(index).Interface()
		} else {
			return nil, false
		}
	}
	return d, true
}
func InterfaceToUint64(d any) (uint64, bool) {
	switch ret := d.(type) {
	case uint64:
		return ret, true
	case uint32:
		return uint64(ret), true
	case uint16:
		return uint64(ret), true
	case uint8:
		return uint64(ret), true
	case int64:
		return uint64(ret), true
	case int32:
		return uint64(ret), true
	case int16:
		return uint64(ret), true
	case int8:
		return uint64(ret), true
	case int:
		return uint64(ret), true
	case float64:
		return uint64(ret), true
	case float32:
		return uint64(ret), true
	}
	return 0, false
}
func GetNodeByPath(node *Node, key string) *Node {
	splits := strings.Split(key, ".")
	var findChildByPath func(node *Node, path ...string) *Node
	findChildByPath = func(node *Node, path ...string) *Node {
		if len(path) == 0 {
			return node
		}
		var child1 *Node
		for _, child := range node.Children {
			if child.Name == path[0] {
				child1 = child
			}
		}
		if child1 == nil {
			return nil
		}
		return findChildByPath(child1, path[1:]...)
	}
	var targetNode *Node
	if strings.HasPrefix(splits[0], "@") {
		splits[0] = splits[0][1:]
		targetNode = findChildByPath(node.Ctx.GetItem("root").(*Node), append([]string{"Package"}, splits...)...)
	} else {
		targetNode = findChildByPath(node, splits...)
	}
	if targetNode == nil {
		return nil
	}
	return targetNode
}

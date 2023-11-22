package stream_parser

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"strings"
)

func appendNode(parent *base.Node, child *base.Node) error {
	err := parent.AppendNode(child)
	if err != nil {
		return err
	}
	return InitNode(utils.GetLastElement(parent.Children))
}
func getNodeByPath(node *base.Node, key string) *base.Node {
	splits := strings.Split(key, ".")
	var findChildByPath func(node *base.Node, path ...string) *base.Node
	findChildByPath = func(node *base.Node, path ...string) *base.Node {
		if len(path) == 0 {
			return node
		}
		var child1 *base.Node
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
	var targetNode *base.Node
	if strings.HasPrefix(splits[0], "@") {
		splits[0] = splits[0][1:]
		targetNode = findChildByPath(node.Ctx.GetItem("root").(*base.Node), append([]string{"Package"}, splits...)...)
	} else {
		targetNode = findChildByPath(node, splits...)
	}
	if targetNode == nil {
		return nil
	}
	return targetNode
}

func getNodeAttrByPath(node *base.Node, key string) (*base.Node, string) {
	splits := strings.Split(key, ".")
	node = getNodeByPath(node, strings.Join(splits[:len(splits)-1], "."))
	return node, splits[len(splits)-1]
}

func getNodeLength(node *base.Node) (uint64, error) {
	var length uint64
	getLengthFaild := false
	if !node.Cfg.Has(CfgLength) {
		itypeName := node.Cfg.GetItem(CfgType)
		if itypeName == nil {
			return 0, errors.New("not set type")
		}
		typeName := utils.InterfaceToString(itypeName)
		switch typeName {
		case "int":
			length = 32
		case "uint":
			length = 32
		case "int8":
			length = 8
		case "uint8":
			length = 8
		case "int16":
			length = 16
		case "uint16":
			length = 16
		case "int32":
			length = 32
		case "uint32":
			length = 32
		case "int64":
			length = 64
		case "uint64":
			length = 64
		default:
			getLengthFaild = true
		}
		//node.Cfg.SetItem("length", length)
	} else {
		length = node.Cfg.GetUint64("length")
	}
	if !getLengthFaild {
		return length, nil
	}
	iparent := node.Cfg.GetItem(CfgParent)
	if iparent == nil {
		return 0, errors.New("not set parentCfg")
	}
	parentNode, ok := iparent.(*base.Node)
	if !ok {
		return 0, errors.New("parentCfg type error")
	}
	if getLengthFaild {
		if parentNode.Cfg.Has(CfgLength) {
			total := parentNode.Cfg.GetUint64(CfgLength)
			nowLength := parentNode.Cfg.GetUint64("now length")
			if nowLength >= total {
				return 0, fmt.Errorf("now length %d greater than total %d", nowLength, total)
			}
			length = total - nowLength
			//node.Cfg.SetItem("length", length)
			getLengthFaild = false
		}
	}
	if getLengthFaild {
		if parentNode.Cfg.Has("length-from-field") {
			fieldName := parentNode.Cfg.GetString("length-from-field")
			for _, child := range parentNode.Children {
				if child.Name == fieldName {
					res, err := GetResultByNode(child)
					if err != nil {
						return 0, err
					}
					if v, ok := base.InterfaceToUint64(res); ok {
						total := v
						if parentNode.Cfg.Has("length-from-field-multiply") {
							mul, ok := base.InterfaceToUint64(parentNode.Cfg.GetItem("length-from-field-multiply"))
							if !ok {
								return 0, fmt.Errorf("length-from-field-multiply type error")
							}
							total *= mul
						}
						nodeRes := parentNode.Cfg.GetItem(CfgNodeResult).(*NodeResult)
						var nowLength uint64
						for _, result := range nodeRes.Sub {
							nowLength += (result.Pos[1] - result.Pos[0])
						}
						if nowLength > total {
							return 0, fmt.Errorf("now length %d greater than total %d", nowLength, total)
						}
						//parentNode.Cfg.SetItem("length", total)
						length = total - nowLength
						//node.Cfg.SetItem("length", length)
						getLengthFaild = false
					} else {
						return 0, fmt.Errorf("field %s type error", fieldName)
					}
					break
				}
			}
		}
	}
	if getLengthFaild {
		return 0, fmt.Errorf("get length faild")
	}
	return length, nil
}

func GetResultByNode(node *base.Node) (any, error) {
	nodeRes := node.Cfg.GetItem(CfgNodeResult).(*NodeResult)
	return nodeRes.Result()
}
func getMapSliceElement(d yaml.MapSlice, path string) any {
	var findChildByPath func(d any, path ...string) any
	findChildByPath = func(d any, path ...string) any {
		if len(path) == 0 {
			return d
		}
		m, ok := d.(yaml.MapSlice)
		if !ok {
			return nil
		}
		var child1 any
		for _, child := range m {
			if child.Key == path[0] {
				child1 = child
			}
		}
		if child1 == nil {
			return nil
		}

		return findChildByPath(child1, path[1:]...)
	}
	return findChildByPath(d, strings.Split(path, ".")...)
}

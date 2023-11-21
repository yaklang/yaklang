package stream_parser

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"reflect"
)

type YakNode struct {
	origin       *base.Node
	Process      func() any
	Name         string
	SetCfg       func(k string, v any)
	AppendNode   func(d *YakNode)
	ForEachChild func(f func(child *YakNode))
	GetParent    func() *YakNode
	GetSubNode   func(name string) *YakNode
}

func ConvertToYakNode(node *base.Node, operator func(node2 *base.Node) error) *YakNode {
	yakNode := &YakNode{}
	yakNode.origin = node
	yakNode.Process = func() any {
		if node.Name == "TCP" {
			print()
		}
		//var err error
		//switch mode {
		//case "parse":
		//	err = node.Parse(data)
		//case "generate":
		//	subData, ok := getSubData(mapData, node.Name)
		//	if !ok {
		//		panic(fmt.Sprintf("sub data %s not found", node.Name))
		//	}
		//	err = node.Generate(subData)
		//}
		err := operator(node)
		if err != nil {
			panic(err)
		}
		res, err := GetNodeResult(node)
		if err != nil {
			panic(fmt.Errorf("parse node %s error: %w", node.Name, err))
		}
		return res
	}
	yakNode.ForEachChild = func(f func(child *YakNode)) {
		for _, child := range node.Children {
			f(ConvertToYakNode(child, operator))
		}
	}
	yakNode.GetParent = func() *YakNode {
		if node.Cfg.Has(CfgParent) {
			parent := node.Cfg.GetItem(CfgParent).(*base.Node)
			return ConvertToYakNode(parent, operator)
		}
		return nil
	}
	yakNode.GetSubNode = func(name string) *YakNode {
		for _, child := range node.Children {
			if child.Name == name {
				return ConvertToYakNode(child, operator)
			}
		}
		return nil
	}
	yakNode.Name = node.Name
	yakNode.SetCfg = func(k string, v any) {
		node.Cfg.SetItem(k, v)
	}
	yakNode.AppendNode = func(d *YakNode) {
		err := appendNode(node, d.origin)
		if err != nil {
			panic(err)
		}
	}
	return yakNode
}
func ExecOperator(node *base.Node, code string, operator func(node2 *base.Node) error) error {
	//if mode != "parse" && mode != "generate" {
	//	return errors.New("mode must be parse or generate")
	//}
	engineLib := map[string]interface{}{
		"this": ConvertToYakNode(node, operator),
		"len": func(i interface{}) int {
			return reflect.ValueOf(i).Len()
		},
		"setCfg": func(key string, value any) {
			targetNode, key := getNodeAttrByPath(node, key)
			if targetNode == nil {
				panic("node not found")
			}
			targetNode.Cfg.SetItem(key, value)
		},
		"getCfg": func(key string) any {
			targetNode, key := getNodeAttrByPath(node, key)
			if targetNode == nil {
				panic("node not found")
			}
			return targetNode.Cfg.GetItem(key)
		},
		"deleteCfg": func(key string) {
			targetNode, key := getNodeAttrByPath(node, key)
			if targetNode == nil {
				panic("node not found")
			}
			targetNode.Cfg.DeleteItem(key)
		},
		"setCtx": func(key string, value any) {
			node.Ctx.SetItem(key, value)
		},
		"getCtx": func(key string) any {
			return node.Ctx.GetItem(key)
		},
		"deleteCtx": func(key string) {
			node.Ctx.DeleteItem(key)
		},
		"getRootNode": func(key string) any { // 需要处理mapData
			rootMap := node.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)
			if v, ok := rootMap[key]; ok {
				return ConvertToYakNode(v, operator)
			}
			panic("not found root node " + key)
		},
		"dump": spew.Dump,
	}
	engine := antlr4yak.New()
	engine.ImportLibs(engineLib)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return engine.SafeEval(ctx, code)
}

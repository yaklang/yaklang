package stream_parser

import (
	"bytes"
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"reflect"
)

type YakNode struct {
	origin               *base.Node
	Process              func() any
	Result               func() any
	Name                 string
	SetCfg               func(k string, v any)
	GetCfg               func(k string) any
	AppendNode           func(d *YakNode)
	ForEachChild         func(f func(child *YakNode))
	GetParent            func() *YakNode
	GetSubNode           func(name string) *YakNode
	GetRemainingSpace    func() uint64
	CalcNodeResultLength func() uint64
	NewElement           func() *YakNode
	TryProcess           func() (any, map[string]any)
	SetChildren          func([]*YakNode)
	GetChildren          func() []*YakNode
	SetLength            func(l uint64)
}

func ConvertToYakNode(node *base.Node, operator func(node *base.Node) (func(bool), error)) *YakNode {
	yakNode := &YakNode{}
	yakNode.origin = node
	yakNode.SetLength = func(l uint64) {
		if !node.Cfg.Has(CfgUnit) {
			panic("node not has unit")
		}
		unit := node.Cfg.GetString(CfgUnit)
		switch unit {
		case "byte":
			node.Cfg.SetItem(CfgLength, l*8)
		case "bit":
			node.Cfg.SetItem(CfgLength, l)
		default:
			panic("unknown unit " + unit)
		}
	}
	yakNode.SetChildren = func(nodes []*YakNode) {
		for _, node := range nodes {
			yakNode.origin.Children = append(yakNode.origin.Children, node.origin)
		}
	}
	yakNode.GetChildren = func() []*YakNode {
		res := []*YakNode{}
		for _, node := range yakNode.origin.Children {
			res = append(res, ConvertToYakNode(node, operator))
		}
		return res
	}
	yakNode.GetCfg = func(k string) any {
		return node.Cfg.GetItem(k)
	}
	yakNode.Result = func() any {
		return GetResultByNode(node)
	}
	yakNode.NewElement = func() *YakNode {
		if len(node.Children) == 0 {
			panic("get node element error")
		}
		if !node.Cfg.Has("template") {
			node.Cfg.SetItem("template", node.Children[0])
			node.Children = nil
		}
		elementTemplate := node.Cfg.GetItem("template").(*base.Node)
		element := elementTemplate.Copy()
		element.Cfg.SetItem(CfgParent, node)
		node.Children = append(node.Children, element)
		return ConvertToYakNode(element, operator)
	}
	yakNode.TryProcess = func() (result any, response map[string]any) {
		response = map[string]any{
			"Ok":      false,
			"Message": "",
			"Save":    func() {},
		}
		defer func() {
			if e := recover(); e != nil {
				response["Message"] = fmt.Sprintf("%v", e)
			}
		}()
		copyNode := node.Copy()
		copyYakNode := ConvertToYakNode(copyNode, operator)
		//err := appendNode(copyNode, yakNode.origin)
		//if err != nil {
		//	response["Message"] = err.Error()
		//	return
		//}
		deferFun, err := operator(copyNode)
		if err != nil {
			response["Message"] = err.Error()
			return
		}
		response["Ok"] = true
		response["Save"] = func() {
			deferFun(false)
		}
		response["GetNode"] = func() any {
			return copyYakNode
		}
		response["Result"] = copyYakNode.Result()
		response["Recovery"] = func() {
			deferFun(true)
		}
		return copyYakNode.Result(), response
	}
	yakNode.Process = func() any {
		//defer func() {
		//	if e := recover(); e != nil {
		//		utils.PrintCurrentGoroutineRuntimeStack()
		//	}
		//}()
		deferFun, err := operator(node)
		if err != nil {
			panic(err)
		}
		deferFun(false)
		return GetResultByNode(node)
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
		if name == "Other" {
			println()
		}
		for _, child := range node.Children {
			if child.Name == name {
				return ConvertToYakNode(child, operator)
			}
		}
		panic(spew.Sprintf("node %s not found", name))
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
	yakNode.GetRemainingSpace = func() uint64 {
		res, err := getNodeLength(yakNode.origin)
		if err != nil {
			panic(err)
		}
		return res
	}
	yakNode.CalcNodeResultLength = func() uint64 {
		return CalcNodeResultLength(yakNode.origin)
	}
	return yakNode
}
func ExecOperator(node *base.Node, code string, operator func(node *base.Node) (func(bool), error)) error {
	//if mode != "parse" && mode != "generate" {
	//	return errors.New("mode must be parse or generate")
	//}
	engineLib := map[string]interface{}{
		"this": ConvertToYakNode(node, operator),
		"len": func(i interface{}) int {
			return reflect.ValueOf(i).Len()
		},
		"getNodeResult": func(key string) any {
			targetNode := getNodeByPath(node, key)
			if targetNode == nil {
				panic("node not found")
			}
			return GetResultByNode(targetNode)
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
		"getNode": func(key string) any {
			n := base.GetNodeByPath(node, key)
			return ConvertToYakNode(n, operator)
		},
		"getCurrentPosition": func() int {
			buf := node.Ctx.GetItem("buffer").(*bytes.Buffer)
			return len(buf.Bytes())
		},
		"dump": spew.Dump,
	}
	engine := antlr4yak.New()
	engine.ImportLibs(engineLib)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return engine.SafeEval(ctx, code)
}

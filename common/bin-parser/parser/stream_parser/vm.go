package stream_parser

import (
	"bytes"
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/utils"
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
	TryProcess           func(*YakNode) (any, map[string]any)
	SetLength            func(l uint64)

	SetChildren      func([]*YakNode)
	GetChildren      func() []*YakNode
	Length           func(uint ...string) uint64
	SetMaxLength     func(l uint64, uint ...string)
	GetMaxLength     func(uint ...string) uint64
	NewSubNode       func(datas ...any) *YakNode
	NewUnknownNode   func(name ...string) *YakNode
	ProcessSubNode   func(name string) any
	ProcessByType    func(datas ...any) any
	TryProcessByType func(datas ...string) (any, map[string]any)
}

func ConvertToYakNode(node *base.Node, operator func(node *base.Node) (func(bool), error)) *YakNode {
	getRootNode := func(key string) *YakNode { // 需要处理mapData
		rootMap := node.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)
		if v, ok := rootMap[key]; ok {
			return ConvertToYakNode(v, operator)
		}
		panic("not found root node " + key)
	}
	yakNode := &YakNode{}
	yakNode.origin = node
	yakNode.ProcessSubNode = func(name string) any {
		return yakNode.GetSubNode(name).Process()
	}
	yakNode.GetMaxLength = func(uints ...string) uint64 {
		n := getDivisor(yakNode, uints)
		l, err := getNodeLength(yakNode.origin)
		if err != nil {
			panic(err)
		}
		return l / n
	}
	yakNode.NewUnknownNode = func(datas ...string) *YakNode {
		name := utils.InterfaceToString(utils.GetLastElement(datas))
		unknownNode := ConvertToYakNode(&base.Node{
			Name:   "Unknown",
			Origin: "raw",
			Cfg:    base.NewConfig(yakNode.origin.Cfg),
			Ctx:    yakNode.origin.Ctx,
		}, operator)
		err := appendNode(node, unknownNode.origin)
		if err != nil {
			panic(err)
		}
		if name != "" {
			utils.GetLastElement(node.Children).Name = name
		}
		return ConvertToYakNode(utils.GetLastElement(node.Children), operator)
	}
	yakNode.SetMaxLength = func(l uint64, uints ...string) {
		n := getDivisor(yakNode, uints)
		node.Cfg.SetItem(CfgLength, l*uint64(n))
	}
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
	yakNode.ProcessByType = func(datas ...any) any {
		var typeName, nodeName string
		switch len(datas) {
		case 1:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = typeName
		case 2:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = utils.InterfaceToString(datas[2])
		default:
			panic("invalid args")
		}
		typeNode := getRootNode(typeName)
		yakNode.AppendNode(typeNode)
		target := utils.GetLastElement(yakNode.origin.Children)
		target.Name = nodeName
		return ConvertToYakNode(target, operator).Process()
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
	yakNode.TryProcessByType = func(datas ...string) (result any, response map[string]any) {
		var typeName, nodeName string
		switch len(datas) {
		case 1:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = typeName
		case 2:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = utils.InterfaceToString(datas[2])
		default:
			panic("invalid args")
		}
		typeNode := getRootNode(typeName)
		response = map[string]any{
			"OK":      false,
			"Message": "",
			"Save":    func() {},
		}
		defer func() {
			if e := recover(); e != nil {
				response["Message"] = fmt.Sprintf("%v", e)
			}
		}()

		//copyNode := node.origin.Copy()
		//copyYakNode := ConvertToYakNode(copyNode, operator)
		yakNode.AppendNode(typeNode)
		copyNode := yakNode.origin.Children[len(yakNode.origin.Children)-1]
		if nodeName != "" {
			copyNode.Name = nodeName
		}
		copyYakNode := ConvertToYakNode(copyNode, operator)
		//err := appendNode(copyNode, yakNode.origin)
		//if err != nil {
		//	response["Message"] = err.Error()
		//	return
		//}
		deferFun, err := operator(copyNode)
		if err != nil {
			response["Message"] = err.Error()
			response["OK"] = false
		} else {
			response["OK"] = true
		}

		response["Save"] = func() {
			deferFun(false)
		}
		response["GetNode"] = func() any {
			return copyYakNode
		}
		response["Result"] = copyYakNode.Result()
		response["Recovery"] = func() {
			deferFun(true)
			yakNode.origin.Children = yakNode.origin.Children[:len(yakNode.origin.Children)-1]
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
	yakNode.NewSubNode = func(datas ...any) *YakNode {
		var typeName, nodeName string
		switch len(datas) {
		case 1:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = typeName
		case 2:
			typeName = utils.InterfaceToString(datas[0])
			nodeName = utils.InterfaceToString(datas[2])
		default:
			panic("invalid args")
		}
		typeNode := getRootNode(typeName)
		err := appendNode(node, typeNode.origin)
		if err != nil {
			panic(err)
		}
		utils.GetLastElement(node.Children).Name = nodeName
		return ConvertToYakNode(utils.GetLastElement(node.Children), operator)
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
	yakNode.Length = func(uints ...string) uint64 {
		n := getDivisor(yakNode, uints)
		return CalcNodeResultLength(yakNode.origin) / uint64(n)
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
func getDivisor(node *YakNode, uints []string) uint64 {
	var uint string
	if len(uints) > 0 {
		uint = utils.InterfaceToString(utils.GetLastElement(uints))
	}
	if uint == "" && node.GetCfg(CfgUnit) != nil {
		uint = node.GetCfg(CfgUnit).(string)
	}
	if uint == "" {
		uint = "byte"
	}
	n := 0
	switch uint {
	case "byte":
		n = 8
	case "bit":
		n = 1
	default:
		panic("unknown unit " + uint)
	}
	return uint64(n)
}

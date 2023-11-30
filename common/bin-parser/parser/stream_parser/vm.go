package stream_parser

import (
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
	Name                 string
	SetCfg               func(k string, v any)
	AppendNode           func(d *YakNode)
	ForEachChild         func(f func(child *YakNode))
	GetParent            func() *YakNode
	GetSubNode           func(name string) *YakNode
	GetRemainingSpace    func() uint64
	CalcNodeResultLength func() uint64
	TryProcessNode       func(*YakNode) map[string]any
}

func ConvertToYakNode(node *base.Node, operator func(node2 *base.Node) error) *YakNode {
	yakNode := &YakNode{}
	yakNode.origin = node
	yakNode.TryProcessNode = func(yNode *YakNode) (response map[string]any) {
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
		copyNodeIns := *node
		copyNode := &copyNodeIns
		err := appendNode(copyNode, yNode.origin)
		if err != nil {
			response["Message"] = err.Error()
			return
		}
		err = operator(utils.GetLastElement[*base.Node](copyNode.Children))
		if err != nil {
			response["Message"] = err.Error()
			return
		}
		response["Ok"] = true
		response["Save"] = func() {
			*node = *copyNode
		}
		return
	}
	yakNode.Process = func() any {
		//defer func() {
		//	if e := recover(); e != nil {
		//		utils.PrintCurrentGoroutineRuntimeStack()
		//	}
		//}()

		err := operator(node)
		if err != nil {
			panic(err)
		}
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
	yakNode.GetRemainingSpace = func() uint64 {
		res, err := getRemainingSpace(yakNode.origin)
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
func ExecOperator(node *base.Node, code string, operator func(node2 *base.Node) error) error {
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
		"dump": spew.Dump,
	}
	engine := antlr4yak.New()
	engine.ImportLibs(engineLib)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return engine.SafeEval(ctx, code)
}

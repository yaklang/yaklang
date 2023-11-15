package _default

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"reflect"
)

func ExecOperator(data *base.BitReader, mapData any, node *base.Node, code string, mode string) (*base.NodeResult, error) {
	if mode != "parse" && mode != "generate" {
		return nil, errors.New("mode must be parse or generate")
	}
	result := base.NewNodeResultByNode(node)
	var generateNodeLib func(node *base.Node) map[string]any
	generateNodeLib = func(node *base.Node) map[string]any {
		thisNodeLib := map[string]any{}
		thisNodeLib["Process"] = func() any {
			iparent := node.Cfg.GetItem("parent")
			var parent *base.Node
			if v, ok := iparent.(*base.Node); ok {
				parent = v
			} else {
				panic("parent not found")
			}
			l, err := getNodeLength(node)
			if err != nil {
				return fmt.Errorf("get node length error: %w", err)
			}
			if l == 0 {
				return nil
			}
			var res *base.NodeResult
			switch mode {
			case "parse":
				res, err = node.Parse(data)
			case "generate":
				subData, ok := getSubData(mapData, node.Name)
				if !ok {
					panic(fmt.Sprintf("sub data %s not found", node.Name))
				}
				res, err = node.Generate(subData)
			}
			if res == nil {
				return nil
			}
			if err != nil {
				panic(err)
			}
			err = result.AppendChild(res)
			if err != nil {
				panic(err)
			}
			parent.Cfg.SetItem("now length", result.Length)
			if res.IsTerminalData {
				n, ok := base.InterfaceToUint64(res.TerminalData)
				if ok {
					return int(n)
				} else {
					return res.TerminalData
				}
			} else {
				return res.Bytes()
			}
		}
		subNodesMap := make(map[string]any)
		for _, child := range node.Children {
			child := child
			sublib := generateNodeLib(child)
			subNodesMap[child.Name] = sublib
			thisNodeLib[child.Name] = sublib
		}
		thisNodeLib["SubNodesMap"] = subNodesMap
		thisNodeLib["SetCfg"] = func(k string, v any) {
			node.Cfg.SetItem(k, v)
		}
		return thisNodeLib
	}

	engineLib := map[string]interface{}{
		"this": generateNodeLib(node),
		"len": func(i interface{}) int {
			return reflect.ValueOf(i).Len()
		},
		"setCfg": func(key string, value any) {
			targetNode, key := getNodeByPath(node, key)
			if targetNode == nil {
				panic("node not found")
			}
			targetNode.Cfg.SetItem(key, value)
		},
		"getCfg": func(key string) any {
			targetNode, key := getNodeByPath(node, key)
			if targetNode == nil {
				panic("node not found")
			}
			return targetNode.Cfg.GetItem(key)
		},
		"deleteCfg": func(key string) {
			targetNode, key := getNodeByPath(node, key)
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
	}
	engine := antlr4yak.New()
	engine.ImportLibs(engineLib)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return result, engine.SafeEval(ctx, code)
}

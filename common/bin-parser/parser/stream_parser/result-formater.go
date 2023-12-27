package stream_parser

import (
	"errors"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

var formatters = map[string]func(node *base.Node) (*base.NodeValue, error){}

func init() {
	formatters["default"] = ToMap
}
func ToMap(node *base.Node) (*base.NodeValue, error) {
	isPackage := func(node *base.Node) bool {
		if node.Name == "Package" && node.Cfg.GetItem(CfgParent) == node.Ctx.GetItem("root") {
			return true
		}
		return false
	}
	if NodeHasResult(node) {
		return newNodeValue(node.Name, GetResultByNode(node)), nil
	}
	if node.Cfg.GetBool(CfgIsList) {
		res := newListNodeValue(node.Name)
		for _, sub := range node.Children {
			d, err := sub.Result()
			if err != nil {
				if errors.Is(err, noResultError) {
					continue
				}
				return nil, err
			}
			res.AppendSub(d)
		}
		if len(res.Children()) == 0 {
			return nil, noResultError
		}
		return res, nil
	} else {
		res := newStructNodeValue(node.Name)
		//res := map[string]any{}
		var getSubs func(node *base.Node) []*base.Node
		getSubs = func(node *base.Node) []*base.Node {
			children := []*base.Node{}
			for _, sub := range node.Children {
				if sub.Cfg.GetBool(CfgIsRefType) || sub.Cfg.GetBool("unpack") || isPackage(sub) {
					children = append(children, getSubs(sub)...)
				} else {
					children = append(children, sub)
				}
			}
			return children
		}
		children := getSubs(node)
		for _, sub := range children {
			d, err := sub.Result()
			if err != nil {
				if errors.Is(err, noResultError) {
					continue
				}
				return nil, err
			}
			res.AppendSub(d)
			//res[sub.Name] = d
		}
		if len(res.Children()) == 0 {
			return nil, noResultError
		}
		return res, nil
	}
}

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
		// A processed, empty list can carry an exact zero-width wire span.
		// Preserve its collection type; dormant lists without a result still
		// produce noResultError below, and terminal raw values stay scalar.
		if node.Cfg.GetBool(CfgIsList) && !NodeIsTerminal(node) && len(node.Children) == 0 {
			span := GetNodeResultPos(node)
			if span[0] == span[1] {
				if _, err := getNodeResult(node, true); err != nil {
					return nil, err
				}
				return newListNodeValue(node), nil
			}
		}
		return newNodeValue(node, GetResultByNode(node)), nil
	}
	if node.Cfg.GetBool(CfgIsList) {
		res := newListNodeValue(node)
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
		res := newStructNodeValue(node)
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

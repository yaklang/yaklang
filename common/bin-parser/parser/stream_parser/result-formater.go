package stream_parser

import (
	"errors"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"golang.org/x/exp/maps"
)

var formatters = map[string]func(node *base.Node) (any, error){}

func init() {
	formatters["default"] = ToMap
}
func ToMap(node *base.Node) (any, error) {
	if node.Cfg.Has("verboseFn") {
		fnCode := node.Cfg.GetString("verboseFn")
		return ExecVerboseFn(node, fnCode)
	}
	isPackage := func(node *base.Node) bool {
		if node.Name == "Package" && node.Cfg.GetItem(CfgParent) == node.Ctx.GetItem("root") {
			return true
		}
		return false
	}
	if NodeHasResult(node) {
		return GetResultByNode(node), nil
	}
	if node.Cfg.GetBool(CfgIsList) {
		var res []any
		for _, sub := range node.Children {
			d, err := sub.Result()
			if err != nil {
				if errors.Is(err, noResultError) {
					continue
				}
				return nil, err
			}
			if v, ok := d.(map[string]any); ok {
				res = append(res, v[maps.Keys(v)[0]])
			} else {
				res = append(res, d)
			}
		}
		if len(res) == 0 {
			return nil, noResultError
		}
		return res, nil
	} else {
		//res := map[string]any{}
		res := map[string]any{}
		var getSubs func(node *base.Node) []*base.Node
		getSubs = func(node *base.Node) []*base.Node {
			children := []*base.Node{}
			for _, sub := range node.Children {
				if sub.Cfg.GetBool("isRefType") || sub.Cfg.GetBool("unpack") || isPackage(sub) {
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
			res[sub.Name] = d
		}
		if len(res) == 0 {
			return nil, noResultError
		}
		return res, nil
	}
}

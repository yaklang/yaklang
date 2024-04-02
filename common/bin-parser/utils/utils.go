package utils

import (
	"bytes"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"strings"
)

func NodeToData(node *base.Node) any {
	if node.Cfg.Has(stream_parser.CfgNodeResult) {
		return stream_parser.GetResultByNode(node)
	}
	if node.Cfg.GetBool(stream_parser.CfgIsList) {
		res := []any{}
		for _, sub := range node.Children {
			d := NodeToData(sub)
			if d != nil {
				res = append(res, d)
			}
		}
		if len(res) == 0 {
			return nil
		}
		return res
	} else {
		res := map[string]any{}
		for _, sub := range node.Children {
			d := NodeToData(sub)
			if d != nil {
				res[sub.Name] = NodeToData(sub)
			}
		}
		if len(res) == 0 {
			return nil
		}
		return res
	}
}
func NodeToBytes(node *base.Node) []byte {
	buffer := node.Ctx.GetItem("buffer").(*bytes.Buffer)
	return buffer.Bytes()
}
func GetSubNode(node *base.Node, path string) *base.Node {
	splits := strings.Split(path, "/")
	if len(splits) == 0 {
		return node
	}
	var getSubNode func(node *base.Node, path []string) *base.Node
	getSubNode = func(node *base.Node, path []string) *base.Node {
		if len(path) == 0 {
			return node
		}

		for _, sub := range stream_parser.GetSubNodes(node) {
			if sub.Name == path[0] {
				return getSubNode(sub, path[1:])
			}
		}
		return nil
	}
	return getSubNode(node, splits)
}
func GetUint64FromMap(d any, key string) uint64 {
	mapData, ok := d.(map[string]any)
	if !ok {
		return 0
	}
	v, ok := mapData[key]
	if !ok {
		return 0
	}
	if v1, ok := v.(uint64); ok {
		return v1
	}
	return 0
}

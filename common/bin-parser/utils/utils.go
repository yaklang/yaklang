package utils

import (
	"bytes"
	"errors"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"reflect"
	"strings"
)

func NodeToStruct(node *base.Node, v any) error {
	refV := reflect.ValueOf(v)
	return nodeToRefV(node, refV)
}
func nodeToRefV(node *base.Node, refV reflect.Value) error {
	if refV.Kind() == reflect.Ptr {
		refV = refV.Elem()
	}
	if node.Cfg.Has(stream_parser.CfgNodeResult) {
		v := reflect.ValueOf(stream_parser.GetResultByNode(node))
		if refV.Kind() != v.Kind() {
			if refV.Kind() == reflect.Array && v.Kind() == reflect.Slice {
				arrayV := reflect.New(refV.Type()).Elem()
				for i := 0; i < v.Len(); i++ {
					arrayV.Index(i).Set(v.Index(i))
				}
				refV.Set(arrayV)
				return nil
			} else if refV.Kind() == reflect.Slice && v.Kind() == reflect.Array {
				sliceV := reflect.New(reflect.SliceOf(refV.Elem().Type()))
				for i := 0; i < v.Len(); i++ {
					reflect.Append(sliceV, v.Index(i))
				}
				refV.Set(sliceV)
				return nil
			} else {
				return errors.New("type not match")
			}
		} else {
			refV.Set(v)
		}
	}
	if node.Cfg.GetBool(stream_parser.CfgIsList) {
		for i, sub := range node.Children {
			field := refV.Index(i)
			err := nodeToRefV(sub, field)
			if err != nil {
				return err
			}
		}
		return nil
	} else {
		for _, sub := range node.Children {
			field := refV.FieldByName(sub.Name)
			err := nodeToRefV(sub, field)
			if err != nil {
				return err
			}
		}
		return nil
	}
}
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

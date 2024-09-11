package fuzzx

import (
	"fmt"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
)

func QuickMutateSimple(target ...string) []string {
	var finalResults []string
	for _, targetItem := range target {
		retResults, err := mutate.QuickMutate(targetItem, consts.GetGormProfileDatabase())
		if err != nil {
			finalResults = append(finalResults, targetItem)
			continue
		}
		finalResults = append(finalResults, retResults...)
	}
	return finalResults
}

func walkJson(value []byte, callback func(key, val gjson.Result, jsonPath string)) {
	_walkGJson(gjson.ParseBytes(value), "", "$", callback)
}

func _walkGJson(value gjson.Result, gPrefix string, jPrefix string, call func(key, val gjson.Result, jPath string)) {
	// 遍历当前层级的所有键
	value.ForEach(func(key, val gjson.Result) bool {
		var jPath string
		// json path syntax
		if key.Type == gjson.Number {
			jPath = fmt.Sprintf("%s[%d]", jPrefix, key.Int())
		} else {
			jPath = fmt.Sprintf("%s.%s", jPrefix, key.String())
		}
		if key.Type == gjson.String && mutate.HasSpecialJSONPathChars(key.String()) {
			jPath = fmt.Sprintf(`%s["%s"]`, jPrefix, key.String())
		}
		// gjson path syntax
		gPath := key.String()
		if gPrefix != "" {
			curr := key.String()
			if mutate.HasSpecialJSONPathChars(key.String()) {
				curr = "\\" + key.String()
			}
			gPath = gPrefix + "." + curr
		}

		call(key, val, jPath)

		// 如果当前值是对象或数组，递归遍历
		if val.IsObject() || val.IsArray() {
			_walkGJson(val, gPath, jPath, call)
		}

		return true
	})
}

func RecursiveXMLNode(node *xmlquery.Node, callback func(node *xmlquery.Node)) {
	nodesMap := make(map[*xmlquery.Node]struct{})
	count := 0

	var recursiveXMLNode func(node *xmlquery.Node)

	recursiveXMLNode = func(node *xmlquery.Node) {
		if node == nil {
			return
		}
		typ := node.Type
		lowerData := strings.ToLower(node.Data)
		lowerPrefix := strings.ToLower(node.Prefix)
		if typ == xmlquery.CommentNode || typ == xmlquery.DeclarationNode || typ == xmlquery.CharDataNode {
			return
		}

		if _, ok := nodesMap[node]; ok {
			return
		}

		// 防止死循环
		count++
		if count > 81920 {
			return
		}

		// soap的特殊节点直接返回
		if (lowerPrefix == "soap" || lowerPrefix == "soap-env") && (lowerData != "envelope" && lowerData != "body") {
			return
		}

		nodesMap[node] = struct{}{}
		// RootNode或者soap的特殊节点不需要回调
		if typ != xmlquery.DocumentNode && lowerPrefix != "soap" && lowerPrefix != "soap-env" {
			callback(node)
		}

		// 遍历兄弟节点
		for sibling := node.PrevSibling; sibling != nil; sibling = sibling.PrevSibling {
			recursiveXMLNode(sibling)
		}
		for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			recursiveXMLNode(sibling)
		}

		// 遍历子节点
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			recursiveXMLNode(child)
		}
	}

	recursiveXMLNode(node)
}

func GetXpathFromNode(node *xmlquery.Node) string {
	var getXpathFromNode func(node *xmlquery.Node, depth int, path string) string

	getXpathFromNode = func(node *xmlquery.Node, depth int, path string) string {
		if node == nil {
			return ""
		}
		nodeType := node.Type

		if nodeType == xmlquery.CommentNode || nodeType == xmlquery.DeclarationNode || nodeType == xmlquery.CharDataNode {
			return ""
		}

		data := node.Data
		prefix := ""
		switch nodeType {
		case xmlquery.TextNode:
			path = "text()"
		case xmlquery.DocumentNode:
			prefix = "/"
		case xmlquery.ElementNode:
			prefix = data
		case xmlquery.AttributeNode:
			prefix = "@" + data
		}

		hasIndex := false
		if node.PrevSibling != nil {
			count := 0
			for prev := node.PrevSibling; prev != nil; prev = prev.PrevSibling {
				if prev.Type == node.Type && prev.Data == data {
					count++
				}
			}
			if count > 0 {
				prefix = fmt.Sprintf("%s[%d]", prefix, count+1)
				hasIndex = true
			}
		}

		if !hasIndex && node.NextSibling != nil {
			existed := false
			for next := node.NextSibling; next != nil; next = next.NextSibling {
				if next.Type == node.Type && next.Data == data {
					existed = true
					break
				}
			}
			if existed {
				prefix = fmt.Sprintf("%s[1]", prefix)
			}
		}

		if prefix != "" {
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			path = prefix + path
		}

		if depth < 128 && node.Parent != nil {
			path = getXpathFromNode(node.Parent, depth+1, path)
		}

		return strings.TrimRight(path, "/")
	}

	return getXpathFromNode(node, 0, "")
}

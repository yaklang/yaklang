package xhtml

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/html"
)

type MatchType string

const (
	TEXT    MatchType = "TEXT"
	COMMENT MatchType = "COMMENT"
	ATTR    MatchType = "ATTR"
)

type MatchNodeInfo struct {
	Xpath           string
	TagName         string
	MatchNode       *html.Node
	MatchText       string
	matchType       MatchType
	Key, Val, Quote string
}

func (m *MatchNodeInfo) IsText() bool {
	if m.matchType == TEXT {
		return true
	}
	return false
}

func (m *MatchNodeInfo) IsAttr() bool {
	if m.matchType == ATTR {
		return true
	}
	return false
}

func (m *MatchNodeInfo) IsCOMMENT() bool {
	if m.matchType == COMMENT {
		return true
	}
	return false
}

func Node2Raw(node *html.Node) string {
	var rendered bytes.Buffer
	err := html.Render(&rendered, node)
	if err != nil {
		return ""
	}
	return string(rendered.Bytes())
}

// Find 解析并遍历一段 HTML 代码的每一个节点并找到匹配字符串的节点，返回匹配字符串的节点信息的引用切片
// Example:
// ```
// matchInfoRes = xhtml.Find("<html><body><div>hello</div></body></html>", "hello")
// ```
func FindNodeFromHtml(htmlRaw interface{}, matchStr string) []*MatchNodeInfo {
	matchInfoRes := []*MatchNodeInfo{}
	// htmlRawStr := utils.InterfaceToString(htmlRaw)
	Walker(htmlRaw, func(node *html.Node) {
		if utils.MatchAllOfGlob(node.Data, fmt.Sprintf("*%s*", matchStr)) {
			if node.Type == html.TextNode {
				matchInfo := &MatchNodeInfo{TagName: node.Parent.Data, MatchNode: node.Parent, MatchText: node.Data, Xpath: GenerateXPath(node.Parent) + "/text()", matchType: TEXT}
				// matchInfo.Raw = Node2Raw(matchInfo.MatchNode)
				matchInfoRes = append(matchInfoRes, matchInfo)
			} else if node.Type == html.CommentNode {
				matchInfo := &MatchNodeInfo{TagName: node.Parent.Data, MatchNode: node.Parent, MatchText: node.Data, Xpath: GenerateXPath(node.Parent) + "/comment()", matchType: COMMENT}
				// matchInfo.Raw = Node2Raw(matchInfo.MatchNode)
				matchInfoRes = append(matchInfoRes, matchInfo)
			}
		} else if node.Type == html.ElementNode {
			for _, attr := range node.Attr {
				if utils.MatchAllOfGlob(attr.Val, fmt.Sprintf("*%s*", matchStr)) {
					matchInfo := &MatchNodeInfo{TagName: node.Data, MatchNode: node, MatchText: attr.Val, Xpath: GenerateXPath(node) + "/@" + attr.Key, matchType: ATTR}
					matchInfo.Key = attr.Key
					matchInfo.Val = attr.Val
					// pattern := ""
					xpathSplits := strings.Split(matchInfo.Xpath, "/")
					for _, xpathSplit := range xpathSplits {
						println(xpathSplit)
					}
					// pattern
					// matchInfo.Raw = Node2Raw(matchInfo.MatchNode)
					matchInfoRes = append(matchInfoRes, matchInfo)
				}
			}
		}
	})
	return matchInfoRes
}

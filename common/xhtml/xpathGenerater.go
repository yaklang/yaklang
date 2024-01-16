package xhtml

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/go-funk"
	"golang.org/x/net/html"
)

func _genXPATHForSimpleNode(origin *html.Node) string {
	siblingN := 0
	psibling := origin
	for {
		psibling = psibling.PrevSibling
		if psibling == nil {
			break
		}
		if psibling.Type == html.ElementNode && psibling.Data == origin.Data {
			siblingN++
		}
	}
	index := siblingN + 1
	psibling = origin
	for {
		psibling = psibling.NextSibling
		if psibling == nil {
			break
		}
		if psibling.Type == html.ElementNode && psibling.Data == origin.Data {
			siblingN++
		}
	}
	if siblingN > 0 {
		return fmt.Sprintf("%v[%v]", origin.Data, index)
	} else {
		return fmt.Sprintf("%v", origin.Data)
	}
}

// GenerateXPath 根据节点引用生成一个节点的 XPath 路径
// Example:
// ```
// xhtml.Walker("<html><body><div>hello</div></body></html>", func(node) {
// println(xhtml.GenerateXPath(node))
// })
// ```
func GenerateXPath(node *html.Node) string {
	var xpath string
	switch node.Type {
	case html.TextNode:
		xpath = generateXPath(node.Parent)
		xpath += "/text()"
	case html.ElementNode:
		xpath = generateXPath(node)
		xpath += "/" + node.Data
	case html.CommentNode:
		xpath = generateXPath(node.Parent)
		xpath += "/Comment()"
	}
	return xpath
}

func generateXPath(origin *html.Node) string {
	var stack []string
	current := origin
	for {
		if current == nil {
			break
		}
		stack = append(stack, _genXPATHForSimpleNode(current))
		current = current.Parent
	}

	stack = funk.ReverseStrings(stack)
	return strings.Join(stack, "/")
}

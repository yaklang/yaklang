package htmlquery

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"golang.org/x/net/html"
)

// LoadHTMLDocument 解析传入的 HTML 文本，返回根节点结构体引用与错误
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// ```
func LoadHTMLDocument(htmlText any) (*html.Node, error) {
	return Parse(strings.NewReader(codec.AnyToString(htmlText)))
}

// OutputHTML 将传入的节点结构体引用转换为 HTML 文本
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// htmlText = xpath.OutputHTML(doc)
// ```
func outputHTML(doc *html.Node) string {
	return OutputHTML(doc, false)
}

// OutputHTMLSelf 将传入的节点结构体引用转换为 HTML 文本，包含自身节点
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// htmlText = xpath.OutputHTMLSelf(doc)
// ```
func outputHTMLSelf(doc *html.Node) string {
	return OutputHTML(doc, true)
}

var Exports = map[string]interface{}{
	"LoadHTMLDocument":     LoadHTMLDocument,
	"Find":                 Find,
	"FindOne":              FindOne,
	"QueryAll":             QueryAll,
	"Query":                Query,
	"InnerText":            InnerText,
	"SelectAttr":           SelectAttr,
	"ExistedAttr":          ExistsAttr,
	"CreateXPathNavigator": CreateXPathNavigator,

	"OutputHTML":     outputHTML,
	"OutputHTMLSelf": outputHTMLSelf,
}
